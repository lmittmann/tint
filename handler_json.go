package tint

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// JSON separator (comma) tokens written after each attribute or group
	jsonSep     = ","
	jsonTintSep = ansiFaint + "," + ansiReset

	hex = "0123456789abcdef"
)

// NewJSONHandler creates a [slog.Handler] that writes tinted logs to Writer w,
// using the default options. Log records are written like with
// [NewTextHandler] up to the message, followed by the attributes formatted as
// pretty JSON with tab indentation. If opts is nil, the default options are
// used.
func NewJSONHandler(w io.Writer, opts *Options) slog.Handler {
	if opts == nil {
		opts = &Options{}
	}
	opts.setDefaults()

	return &handler{
		json: true,
		mu:   &sync.Mutex{},
		w:    w,
		opts: *opts,
	}
}

// appendJSONAttrs writes the handler and record attributes as a pretty JSON
// object in a new line below the message, ending with a trailing space.
func (h *handler) appendJSONAttrs(buf *buffer, r slog.Record) {
	// start the JSON object in a new line below the message
	if len(*buf) > 0 {
		(*buf)[len(*buf)-1] = '\n' // replace last space with newline
	}

	pos := len(*buf)
	h.appendJSONToken(buf, "{")
	posOpen := len(*buf)

	// write handler attributes
	buf.WriteString(h.attrsPrefix)

	// open groups that are not yet opened by the handler attributes
	posGroups := len(*buf)
	for i := h.nOpenGroups; i < len(h.groups); i++ {
		h.appendJSONGroupOpen(buf, h.groups[i], i+1)
	}

	// write record attributes
	posAttrs := len(*buf)
	r.Attrs(func(attr slog.Attr) bool {
		h.appendAttr(buf, attr, h.groupPrefix, h.groups)
		return true
	})

	nOpenGroups := len(h.groups)
	if len(*buf) == posAttrs {
		*buf = (*buf)[:posGroups] // elide groups without attributes
		nOpenGroups = h.nOpenGroups
	}

	if len(*buf) == posOpen {
		*buf = (*buf)[:pos] // no attributes: don't render the JSON object
		return
	}

	h.trimJSONSep(buf)
	for i := nOpenGroups; i > 0; i-- {
		h.appendJSONGroupClose(buf, i)
	}
	buf.WriteByte('\n')
	h.appendJSONToken(buf, "}")
	buf.WriteByte(' ') // replaced by newline in Handle
}

// appendJSONAttrsPrefix writes the attributes, opening not yet opened groups,
// and returns the new number of open groups.
func (h *handler) appendJSONAttrsPrefix(buf *buffer, attrs []slog.Attr) (nOpenGroups int) {
	pos := len(*buf)
	for i := h.nOpenGroups; i < len(h.groups); i++ {
		h.appendJSONGroupOpen(buf, h.groups[i], i+1)
	}

	posAttrs := len(*buf)
	for _, attr := range attrs {
		h.appendAttr(buf, attr, h.groupPrefix, h.groups)
	}

	if len(*buf) == posAttrs {
		*buf = (*buf)[:pos] // elide groups without attributes
		return h.nOpenGroups
	}
	return len(h.groups)
}

// appendJSONGroupAttr writes a group attribute as a nested JSON object, or
// nothing if the group is empty.
func (h *handler) appendJSONGroupAttr(buf *buffer, attr slog.Attr, groups []string) {
	pos := len(*buf)
	h.appendJSONGroupOpen(buf, attr.Key, len(groups))

	posAttrs := len(*buf)
	for _, groupAttr := range attr.Value.Group() {
		h.appendAttr(buf, groupAttr, "", groups)
	}

	if len(*buf) == posAttrs {
		*buf = (*buf)[:pos] // elide empty group
		return
	}
	h.trimJSONSep(buf)
	h.appendJSONGroupClose(buf, len(groups))
	h.appendJSONSep(buf)
}

// appendJSONGroupOpen writes `"name": {` in a new line with the given
// indentation depth.
func (h *handler) appendJSONGroupOpen(buf *buffer, name string, depth int) {
	buf.WriteByte('\n')
	appendTabs(buf, depth)
	if h.opts.NoColor {
		h.appendJSONKey(buf, name)
		buf.WriteString(" {")
	} else {
		buf.WriteString(ansiFaint)
		h.appendJSONKey(buf, name)
		buf.WriteString(" {")
		buf.WriteString(ansiReset)
	}
}

// appendJSONGroupClose writes `}` in a new line with the given indentation
// depth.
func (h *handler) appendJSONGroupClose(buf *buffer, depth int) {
	buf.WriteByte('\n')
	appendTabs(buf, depth)
	h.appendJSONToken(buf, "}")
}

// appendJSONAttr writes `"key": value` in a new line with the given
// indentation depth and a trailing comma.
func (h *handler) appendJSONAttr(buf *buffer, attr slog.Attr, color int16, depth int) {
	buf.WriteByte('\n')
	appendTabs(buf, depth)

	if h.opts.NoColor {
		h.appendJSONKey(buf, attr.Key)
		buf.WriteByte(' ')
		h.appendJSONValue(buf, attr.Value)
	} else if color >= 0 {
		appendAnsi(buf, uint8(color), true)
		h.appendJSONKey(buf, attr.Key)
		buf.WriteString(ansiResetFaint)
		buf.WriteByte(' ')
		h.appendJSONValue(buf, attr.Value)
		buf.WriteString(ansiReset)
	} else {
		buf.WriteString(ansiFaint)
		h.appendJSONKey(buf, attr.Key)
		buf.WriteString(ansiReset)
		buf.WriteByte(' ')
		h.appendJSONValue(buf, attr.Value)
	}
	h.appendJSONSep(buf)
}

func (h *handler) appendJSONKey(buf *buffer, key string) {
	h.appendJSONString(buf, key)
	buf.WriteByte(':')
}

func (h *handler) appendJSONValue(buf *buffer, v slog.Value) {
	switch v.Kind() {
	case slog.KindString:
		h.appendJSONString(buf, v.String())
	case slog.KindInt64:
		*buf = strconv.AppendInt(*buf, v.Int64(), 10)
	case slog.KindUint64:
		*buf = strconv.AppendUint(*buf, v.Uint64(), 10)
	case slog.KindFloat64:
		// Copied from log/slog/json_handler.go.
		//
		// json.Marshal is funny about floats; it doesn't
		// always match strconv.AppendFloat. So just call it.
		// That's expensive, but floats are rare.
		if err := appendJSONMarshal(buf, v.Float64()); err != nil {
			h.appendJSONString(buf, "!ERROR: "+err.Error())
		}
	case slog.KindBool:
		*buf = strconv.AppendBool(*buf, v.Bool())
	case slog.KindDuration:
		*buf = strconv.AppendInt(*buf, int64(v.Duration()), 10)
	case slog.KindTime:
		buf.WriteByte('"')
		*buf = v.Time().AppendFormat(*buf, time.RFC3339Nano)
		buf.WriteByte('"')
	case slog.KindAny:
		defer func() {
			// Copied from log/slog/handler.go.
			if r := recover(); r != nil {
				// If it panics with a nil pointer, the most likely cases are
				// an encoding.TextMarshaler or error fails to guard against nil,
				// in which case "<nil>" seems to be the feasible choice.
				//
				// Adapted from the code in fmt/print.go.
				if v := reflect.ValueOf(v.Any()); v.Kind() == reflect.Pointer && v.IsNil() {
					h.appendJSONString(buf, "<nil>")
					return
				}

				// Otherwise just print the original panic message.
				h.appendJSONString(buf, fmt.Sprintf("!PANIC: %v", r))
			}
		}()

		// Copied from log/slog/json_handler.go.
		a := v.Any()
		_, jm := a.(json.Marshaler)
		if err, ok := a.(error); ok && !jm {
			h.appendJSONString(buf, err.Error())
		} else if err := appendJSONMarshal(buf, a); err != nil {
			h.appendJSONString(buf, "!ERROR: "+err.Error())
		}
	}
}

// appendJSONString writes s as a quoted JSON string. ANSI escape sequences
// are written as-is if color is enabled, and are stripped otherwise.
func (h *handler) appendJSONString(buf *buffer, s string) {
	if h.opts.NoColor && strings.IndexByte(s, byte(ansiEsc)) >= 0 {
		// trim ANSI escape sequences
		var inEscape bool
		s = cut(s, func(r rune) bool {
			if r == ansiEsc {
				inEscape = true
			} else if inEscape && unicode.IsLetter(r) {
				inEscape = false
				return true
			}

			return inEscape
		})
	}

	buf.WriteByte('"')
	*buf = appendEscapedJSONString(*buf, s)
	buf.WriteByte('"')
}

func appendJSONMarshal(buf *buffer, v any) error {
	// Use a json.Encoder to avoid escaping HTML.
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	*buf = (*buf)[:len(*buf)-1] // remove final newline
	return nil
}

// appendEscapedJSONString escapes s for JSON and appends it to buf.
// It does not surround the string in quotation marks.
//
// Copied from log/slog/json_handler.go, but keeps the ANSI escape code
// "\u001b" unescaped as allowed by safeSet.
func appendEscapedJSONString(buf []byte, s string) []byte {
	char := func(b byte) { buf = append(buf, b) }
	str := func(s string) { buf = append(buf, s...) }

	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if safeSet[b] {
				i++
				continue
			}
			if start < i {
				str(s[start:i])
			}
			char('\\')
			switch b {
			case '\\', '"':
				char(b)
			case '\n':
				char('n')
			case '\r':
				char('r')
			case '\t':
				char('t')
			default:
				// This encodes bytes < 0x20 except for \t, \n and \r.
				str(`u00`)
				char(hex[b>>4])
				char(hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				str(s[start:i])
			}
			str(`\ufffd`)
			i += size
			start = i
			continue
		}
		// U+2028 is LINE SEPARATOR.
		// U+2029 is PARAGRAPH SEPARATOR.
		// They are both technically valid characters in JSON strings,
		// but don't work in JSONP, which has to be evaluated as JavaScript,
		// and can lead to security holes there. It is valid JSON to
		// escape them, so we do so unconditionally.
		// See http://timelessrepo.com/json-isnt-a-javascript-subset for discussion.
		if c == '\u2028' || c == '\u2029' {
			if start < i {
				str(s[start:i])
			}
			str(`\u202`)
			char(hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		str(s[start:])
	}
	return buf
}

// appendJSONSep writes a dimmed comma.
func (h *handler) appendJSONSep(buf *buffer) {
	if h.opts.NoColor {
		buf.WriteString(jsonSep)
	} else {
		buf.WriteString(jsonTintSep)
	}
}

// trimJSONSep removes a trailing comma written by appendJSONSep.
func (h *handler) trimJSONSep(buf *buffer) {
	sep := jsonSep
	if !h.opts.NoColor {
		sep = jsonTintSep
	}
	if b := *buf; len(b) >= len(sep) && string(b[len(b)-len(sep):]) == sep {
		*buf = b[:len(b)-len(sep)]
	}
}

// appendJSONToken writes a dimmed JSON token like "{" or "}".
func (h *handler) appendJSONToken(buf *buffer, token string) {
	if h.opts.NoColor {
		buf.WriteString(token)
	} else {
		buf.WriteString(ansiFaint)
		buf.WriteString(token)
		buf.WriteString(ansiReset)
	}
}

func appendTabs(buf *buffer, n int) {
	for i := 0; i < n; i++ {
		buf.WriteByte('\t')
	}
}
