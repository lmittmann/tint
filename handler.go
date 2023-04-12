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

const keyErr = "err"

var defaultTimeFormat = time.StampMilli

// Options for a slog.Handler that writes tinted logs. A zero Options consists
// entirely of default values.
type Options struct {
	// Minimum level to log (Default: slog.LevelInfo)
	Level slog.Level

	// Time format (Default: time.StampMilli)
	TimeFormat string

	// Disable color (Default: false)
	NoColor bool

	// ReplaceAttr is called to rewrite each non-group attribute before it is logged.
	// The attribute's value has been resolved (see [Value.Resolve]).
	// If ReplaceAttr returns an Attr with Key == "", the attribute is discarded.
	//
	// The built-in attributes with keys "time", "level", "source", and "msg"
	// are passed to this function.
	//
	// The first argument is a list of currently open groups that contain the
	// Attr. It must not be retained or modified. ReplaceAttr is never called
	// for Group attributes, only their contents. For example, the attribute
	// list
	//
	//     Int("a", 1), Group("g", Int("b", 2)), Int("c", 3)
	//
	// results in consecutive calls to ReplaceAttr with the following arguments:
	//
	//     nil, Int("a", 1)
	//     []string{"g"}, Int("b", 2)
	//     nil, Int("c", 3)
	//
	// ReplaceAttr can be used to change the default keys of the built-in
	// attributes, convert types (for example, to replace a `time.Time` with the
	// integer seconds since the Unix epoch), sanitize personal information, or
	// remove attributes from the output.
	//
	// ReplaceAttr can be used to customize formatting of level. If level
	// attribute is replaced by a string value, the string is written as-is to
	// the output.
	ReplaceAttr func(groups []string, attr slog.Attr) slog.Attr
}

// NewHandler creates a [slog.Handler] that writes tinted logs to Writer w with
// the given options.
func (opts Options) NewHandler(w io.Writer) slog.Handler {
	h := &handler{
		w:           w,
		level:       opts.Level,
		timeFormat:  opts.TimeFormat,
		noColor:     opts.NoColor,
		replaceAttr: opts.ReplaceAttr,
	}
	if h.timeFormat == "" {
		h.timeFormat = defaultTimeFormat
	}
	return h
}

// NewHandler creates a [slog.Handler] that writes tinted logs to Writer w,
// using the default options.
func NewHandler(w io.Writer) slog.Handler {
	return Options{}.NewHandler(w)
}

// handler implements a [slog.Handler].
type handler struct {
	attrs       string
	groups      string
	groupsSlice []string

	mu sync.Mutex
	w  io.Writer // Output writer

	level       slog.Level // Minimum level to log (Default: slog.LevelInfo)
	timeFormat  string     // Time format (Default: time.StampMilli)
	noColor     bool       // Disable color (Default: false)
	replaceAttr func([]string, slog.Attr) slog.Attr
}

func (h *handler) clone() *handler {
	return &handler{
		attrs:       h.attrs,
		groups:      h.groups,
		groupsSlice: h.groupsSlice,
		w:           h.w,
		level:       h.level,
		timeFormat:  h.timeFormat,
		noColor:     h.noColor,
		replaceAttr: h.replaceAttr,
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
	a := slog.Time(slog.TimeKey, r.Time)
	if h.replaceAttr != nil {
		a = h.replaceAttr(nil, a)
	}
	if a.Key != "" {
		if a.Value.Kind() == slog.KindString {
			buf.WriteString(a.Value.String())
		} else if a.Value.Kind() == slog.KindTime {
			h.appendTime(buf, a.Value.Time())
		} else {
			appendValue(buf, a.Value)
		}
		buf.WriteByte(' ')
	}

	// write level
	a = slog.Int(slog.LevelKey, int(r.Level))
	if h.replaceAttr != nil {
		a = h.replaceAttr(nil, a)
	}
	if a.Key != "" {
		if a.Value.Kind() == slog.KindString {
			buf.WriteString(a.Value.String())
		} else if a.Value.Kind() == slog.KindInt64 {
			h.appendLevel(buf, r.Level)
		} else {
			appendValue(buf, a.Value)
		}
		buf.WriteByte(' ')
	}

	// write message
	a = slog.String(slog.MessageKey, r.Message)
	if h.replaceAttr != nil {
		a = h.replaceAttr(nil, a)
	}
	buf.WriteString(a.Value.String())

	// write handler attributes
	if len(h.attrs) > 0 {
		buf.WriteString(h.attrs)
	}

	// write attributes
	r.Attrs(func(attr slog.Attr) {
		if h.replaceAttr != nil {
			attr = h.replaceAttr(h.groupsSlice, attr)
		}
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
		if h.replaceAttr != nil {
			attr = h.replaceAttr(h.groupsSlice, attr)
		}
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
	h2.groupsSlice = append(h2.groupsSlice, name)
	return h2
}

func (h *handler) appendTime(buf *buffer, t time.Time) {
	buf.WriteStringIf(!h.noColor, ansiFaint)
	*buf = t.AppendFormat(*buf, h.timeFormat)
	buf.WriteStringIf(!h.noColor, ansiReset)
}

func (h *handler) appendLevel(buf *buffer, level slog.Level) {
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
		buf.WriteStringIf(!h.noColor, ansiBrightGreen)
		buf.WriteString("INF")
		delta(buf, level-slog.LevelInfo)
		buf.WriteStringIf(!h.noColor, ansiReset)
	case level < slog.LevelError:
		buf.WriteStringIf(!h.noColor, ansiBrightYellow)
		buf.WriteString("WRN")
		delta(buf, level-slog.LevelWarn)
		buf.WriteStringIf(!h.noColor, ansiReset)
	default:
		buf.WriteStringIf(!h.noColor, ansiBrightRed)
		buf.WriteString("ERR")
		delta(buf, level-slog.LevelError)
		buf.WriteStringIf(!h.noColor, ansiReset)
	}
}

func (h *handler) appendAttr(buf *buffer, attr slog.Attr, groups string) {
	if attr.Key == "" {
		return
	}

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
	buf.WriteStringIf(!h.noColor, ansiFaint)
	appendString(buf, h.groups+groups+key)
	buf.WriteByte('=')
	buf.WriteStringIf(!h.noColor, ansiReset)
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
	buf.WriteStringIf(!h.noColor, ansiBrightRedFaint)
	appendString(buf, h.groups+groups+keyErr)
	buf.WriteByte('=')
	buf.WriteStringIf(!h.noColor, ansiResetFaint)
	appendString(buf, err.Error())
	buf.WriteStringIf(!h.noColor, ansiReset)
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
func Err(err error) slog.Attr { return slog.Any(keyErr, tintError{err}) }
