/*
Package tint provides a [slog.Handler] that prints tinted logs. The output
format is inspired by the [zerolog.ConsoleWriter].

[zerolog.ConsoleWriter]: https://pkg.go.dev/github.com/rs/zerolog#ConsoleWriter
*/
package tint

import (
	"context"
	"encoding"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
	"unicode"

	"golang.org/x/exp/slog"
)

var (
	defaultOut        = os.Stderr
	defaultLvl        = slog.LevelInfo
	defaultTimeFormat = time.StampMilli

	lvlStrings = map[slog.Level]string{
		slog.LevelDebug: "DBG",
		slog.LevelInfo:  "\033[92mINF\033[0m",
		slog.LevelWarn:  "\033[33mWRN\033[0m",
		slog.LevelError: "\033[91mERR\033[0m",
	}
)

// Handler implements a [slog.Handler].
type Handler struct {
	once   sync.Once
	attrs  []byte
	groups []byte

	mu  sync.Mutex
	Out io.Writer // Output writer (Default: os.Stderr)

	Level      slog.Level // Minimum level to log (Default: slog.InfoLevel)
	TimeFormat string     // Time format (Default: time.StampMilli)
}

// init sets all unset fields to their defaults.
func (h *Handler) init() {
	if h.Out == nil {
		h.Out = defaultOut
	}
	if h.Level < slog.LevelDebug {
		h.Level = defaultLvl
	}
	if h.TimeFormat == "" {
		h.TimeFormat = defaultTimeFormat
	}
}

func (h *Handler) clone() *Handler {
	h2 := &Handler{
		attrs:      h.attrs,
		groups:     h.groups,
		Out:        h.Out,
		Level:      h.Level,
		TimeFormat: h.TimeFormat,
	}
	h2.once.Do(func() {})
	return h2
}

func (h *Handler) Enabled(ctx context.Context, lvl slog.Level) bool {
	h.once.Do(h.init)

	return lvl >= h.Level
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	h.once.Do(h.init)

	// get a buffer from the sync pool
	buf := newBuffer()
	defer buf.Free()

	// write time, level, and message
	buf.WriteString("\033[2m")
	*buf = r.Time.AppendFormat(*buf, h.TimeFormat)
	buf.WriteString("\033[0m ")
	buf.WriteString(lvlStrings[r.Level])
	buf.WriteByte(' ')
	buf.WriteString(r.Message)

	// write handler attributes
	if len(h.attrs) > 0 {
		buf.Write(h.attrs)
	}

	// write attributes
	r.Attrs(func(attr slog.Attr) {
		buf.WriteByte(' ')
		h.appendAttr(buf, attr)
	})
	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := h.Out.Write(*buf)
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.once.Do(h.init)

	if len(attrs) == 0 {
		return h
	}
	h2 := h.clone()

	buf := newBuffer()
	defer buf.Free()

	// write attributes to buffer
	for _, attr := range attrs {
		buf.WriteByte(' ')
		h.appendAttr(buf, attr)
	}
	h2.attrs = append(h.attrs, *buf...)
	return h2
}

func (h *Handler) WithGroup(name string) slog.Handler {
	h.once.Do(h.init)

	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groups = append(append(h2.groups, name...), '.')
	return h2
}

func (h *Handler) appendAttr(buf *buffer, attr slog.Attr) {
	if attr.Value.Kind() == slog.KindAny {
		if err, ok := attr.Value.Any().(tintError); ok {
			// append tintError
			h.appendTintError(buf, err)
			return
		}
	}

	h.appendKey(buf, attr.Key)
	appendValue(buf, attr.Value)
}

func (h *Handler) appendKey(buf *buffer, key string) {
	buf.WriteString("\033[2m")
	if len(h.groups) > 0 {
		buf.Write(h.groups)
	}
	appendString(buf, key)
	buf.WriteString("=\033[0m")
}

func appendValue(buf *buffer, v slog.Value) {
	switch v.Kind() {
	case slog.KindString:
		appendString(buf, v.String())
	case slog.KindInt64:
		*buf = strconv.AppendInt(*buf, v.Int64(), 10)
	case slog.KindUint64:
		*buf = strconv.AppendUint(*buf, v.Uint64(), 10)
	case slog.KindFloat64:
		*buf = strconv.AppendFloat(*buf, v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		*buf = strconv.AppendBool(*buf, v.Bool())
	case slog.KindDuration:
		appendString(buf, v.Duration().String())
	case slog.KindTime:
		appendString(buf, v.Time().String())
	case slog.KindAny:
		if tm, ok := v.Any().(encoding.TextMarshaler); ok {
			data, err := tm.MarshalText()
			if err != nil {
				break
			}
			appendString(buf, string(data))
			break
		}
		appendString(buf, fmt.Sprint(v.Any()))
	}
}

func (h *Handler) appendTintError(buf *buffer, err error) {
	buf.WriteString("\033[91;2m")
	if len(h.groups) > 0 {
		buf.Write(h.groups)
	}
	buf.WriteString("err=\033[22m")
	appendString(buf, err.Error())
	buf.WriteString("\033[0m")
}

func appendString(buf *buffer, s string) {
	if needsQuoting(s) {
		*buf = strconv.AppendQuote(*buf, s)
	} else {
		buf.WriteString(s)
	}
}

func needsQuoting(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) || r == '"' || r == '=' || !unicode.IsPrint(r) {
			return true
		}
	}
	return false
}

type tintError error

// Err returns a tinted slog.Attr.
func Err(err error) slog.Attr { return slog.Any("", tintError(err)) }
