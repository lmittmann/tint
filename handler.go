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

// ANSI modes
const (
	ansiReset          = "\033[0m"
	ansiFaint          = "\033[2m"
	ansiResetFaint     = "\033[22m"
	ansiBrightRed      = "\033[91m"
	ansiBrightGreen    = "\033[92m"
	ansiBrightYellow   = "\033[93m"
	ansiBrightRedFaint = "\033[91;2m"
)

var (
	defaultTimeFormat = time.StampMilli
)

// Options for a slog.Handler that writes tinted logs. A zero Options consists
// entirely of default values.
type Options struct {
	// Minimum level to log (Default: slog.InfoLevel)
	Level slog.Level

	// Time format (Default: time.StampMilli)
	TimeFormat string

	LevelStrings map[slog.Level]string
}

// NewHandler creates a [slog.Handler] that writes tinted logs to w with the
// given options.
func (opts Options) NewHandler(w io.Writer) slog.Handler {
	h := &handler{
		w:            w,
		level:        opts.Level,
		levelStrings: opts.LevelStrings,
		timeFormat:   opts.TimeFormat,
	}
	if h.timeFormat == "" {
		h.timeFormat = defaultTimeFormat
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
	attrs  string
	groups string

	mu sync.Mutex
	w  io.Writer // Output writer

	level        slog.Level            // Minimum level to log (Default: slog.InfoLevel)
	levelStrings map[slog.Level]string // Per level precomputed message prefix.
	timeFormat   string                // Time format (Default: time.StampMilli)
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

	// write time
	h.appendTime(buf, r.Time)
	buf.WriteByte(' ')

	// write level
	h.appendLevel(buf, r.Level)
	buf.WriteByte(' ')

	// write message
	buf.WriteString(r.Message)

	// write handler attributes
	if len(h.attrs) > 0 {
		buf.WriteString(h.attrs)
	}

	// write attributes
	r.Attrs(func(attr slog.Attr) {
		h.appendAttr(buf, attr, "")
	})
	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := h.w.Write(*buf)
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
		h.appendAttr(buf, attr, "")
	}
	h2.attrs = h.attrs + string(*buf)
	return h2
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groups = h2.groups + name + "."
	return h2
}

func (h *handler) appendTime(buf *buffer, t time.Time) {
	buf.WriteString(ansiFaint)
	*buf = t.AppendFormat(*buf, h.timeFormat)
	buf.WriteString(ansiReset)
}

func (h *handler) appendLevel(buf *buffer, level slog.Level) {
	// Use predefined snippet if available. Fallback to computed value.
	levelString, ok := h.levelStrings[level]
	if ok {
		buf.WriteString(levelString)
		return
	}

	delta := func(buf *buffer, val slog.Level) {
		if val == 0 {
			return
		}
		buf.WriteByte('+')
		*buf = strconv.AppendInt(*buf, int64(val), 10)
	}

	switch {
	case level < slog.LevelInfo:
		buf.WriteString("DBG")
		delta(buf, level-slog.LevelDebug)
	case level < slog.LevelWarn:
		buf.WriteString(ansiBrightGreen)
		buf.WriteString("INF")
		delta(buf, level-slog.LevelInfo)
		buf.WriteString(ansiReset)
	case level < slog.LevelError:
		buf.WriteString(ansiBrightYellow)
		buf.WriteString("WRN")
		delta(buf, level-slog.LevelWarn)
		buf.WriteString(ansiReset)
	default:
		buf.WriteString(ansiBrightRed)
		buf.WriteString("ERR")
		delta(buf, level-slog.LevelError)
		buf.WriteString(ansiReset)
	}
}

func (h *handler) appendAttr(buf *buffer, attr slog.Attr, groups string) {
	if kind := attr.Value.Kind(); kind == slog.KindGroup {
		groups = groups + attr.Key + "."
		for _, groupAttr := range attr.Value.Group() {
			h.appendAttr(buf, groupAttr, groups)
		}
		return
	} else if kind == slog.KindAny {
		if err, ok := attr.Value.Any().(tintError); ok {
			// append tintError
			buf.WriteByte(' ')
			h.appendTintError(buf, err, groups)
			return
		}
	}

	buf.WriteByte(' ')
	h.appendKey(buf, attr.Key, groups)
	appendValue(buf, attr.Value)
}

func (h *handler) appendKey(buf *buffer, key string, groups string) {
	buf.WriteString(ansiFaint)
	appendString(buf, h.groups+groups+key)
	buf.WriteString("=" + ansiReset)
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

func (h *handler) appendTintError(buf *buffer, err error, groups string) {
	buf.WriteString(ansiBrightRedFaint)
	appendString(buf, h.groups+groups+"err")
	buf.WriteString("=" + ansiResetFaint)
	appendString(buf, err.Error())
	buf.WriteString(ansiReset)
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

type tintError struct{ error }

// Err returns a tinted slog.Attr.
func Err(err error) slog.Attr { return slog.Any("", tintError{err}) }
