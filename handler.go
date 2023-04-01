/*
Package tint provides a [slog.Handler] that writes tinted logs. The output
format is inspired by the [zerolog.ConsoleWriter].

[zerolog.ConsoleWriter]: https://pkg.go.dev/github.com/rs/zerolog#ConsoleWriter
*/
package tint

import (
	"context"
	"encoding"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
	"unicode"

	"golang.org/x/exp/slog"
)

var (
	defaultLevel      = slog.LevelInfo
	defaultTimeFormat = time.StampMilli

	levelStrings = map[slog.Level]string{
		slog.LevelDebug: "DBG",
		slog.LevelInfo:  "\033[92mINF\033[0m",
		slog.LevelWarn:  "\033[93mWRN\033[0m",
		slog.LevelError: "\033[91mERR\033[0m",
	}
)

// Options for a slog.Handler that writes tinted logs. A zero Options consists
// entirely of default values.
type Options struct {
	// Minimum level to log (Default: slog.InfoLevel)
	Level slog.Level

	// Time format (Default: time.StampMilli)
	TimeFormat string

	// Specifies the function to run when write
	OnWrite func(level slog.Level, message string)
}

// NewHandler creates a [slog.Handler] that writes tinted logs to w with the
// given options.
func (opts Options) NewHandler(w io.Writer) slog.Handler {
	h := &handler{
		w:          w,
		level:      opts.Level,
		timeFormat: opts.TimeFormat,
	}
	if h.level < slog.LevelDebug {
		h.level = defaultLevel
	}
	if h.timeFormat == "" {
		h.timeFormat = defaultTimeFormat
	}
	if opts.OnWrite != nil {
		h.onWrite = opts.OnWrite
	}
	return h
}

// NewHandler creates a [slog.Handler] that writes tinted logs to w, using the default
// options.
func NewHandler(w io.Writer) slog.Handler {
	return (Options{}).NewHandler(w)
}

// handler implements a [slog.handler].
type handler struct {
	attrs  []byte
	groups []byte

	mu sync.Mutex
	w  io.Writer // Output writer

	level      slog.Level // Minimum level to log (Default: slog.InfoLevel)
	timeFormat string     // Time format (Default: time.StampMilli)

	// function executed in a new goroutine at each write
	onWrite func(level slog.Level, message string)
}

func (h *handler) clone() *handler {
	return &handler{
		attrs:      h.attrs,
		groups:     h.groups,
		w:          h.w,
		level:      h.level,
		timeFormat: h.timeFormat,
	}
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	// get a buffer from the sync pool
	buf := newBuffer()
	defer buf.Free()

	// write time, level, and message
	buf.WriteString("\033[2m")
	*buf = r.Time.AppendFormat(*buf, h.timeFormat)
	buf.WriteString("\033[0m ")
	buf.WriteString(levelStrings[r.Level])
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

	_, err := h.w.Write(*buf)

	if h.onWrite != nil {
		go h.onWrite(r.Level, r.Message)
	}

	return err
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
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

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groups = append(append(h2.groups, name...), '.')
	return h2
}

func (h *handler) appendAttr(buf *buffer, attr slog.Attr) {
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

func (h *handler) appendKey(buf *buffer, key string) {
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

func (h *handler) appendTintError(buf *buffer, err error) {
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
