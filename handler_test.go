package tint_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lmittmann/tint"
)

func Example() {
	w := os.Stderr
	logger := slog.New(tint.NewHandler(w, &tint.Options{
		Level:      slog.LevelDebug,
		TimeFormat: time.Kitchen,
	}))
	logger.Info("Starting server", "addr", ":8080", "env", "production")
	logger.Debug("Connected to DB", "db", "myapp", "host", "localhost:5432")
	logger.Warn("Slow request", "method", "GET", "path", "/users", "duration", 497*time.Millisecond)
	logger.Error("DB connection lost", tint.Err(errors.New("connection reset")), "db", "myapp")
	// Output:
}

// Create a new logger that writes all errors in red:
func Example_redErrors() {
	w := os.Stderr
	logger := slog.New(tint.NewHandler(w, &tint.Options{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindAny {
				if _, ok := a.Value.Any().(error); ok {
					return tint.Attr(9, a)
				}
			}
			return a
		},
	}))
	logger.Error("DB connection lost", "error", errors.New("connection reset"), "db", "myapp")
	// Output:
}

// Create a new logger with a custom TRACE level:
func Example_traceLevel() {
	const LevelTrace = slog.LevelDebug - 4

	w := os.Stderr
	logger := slog.New(tint.NewHandler(w, &tint.Options{
		Level: LevelTrace,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey && len(groups) == 0 {
				level, ok := a.Value.Any().(slog.Level)
				if ok && level <= LevelTrace {
					return tint.Attr(13, slog.String(a.Key, "TRC"))
				}
			}
			return a
		},
	}))
	logger.Log(context.Background(), LevelTrace, "DB query", "query", "SELECT * FROM users", "duration", 543*time.Microsecond)
	// Output:
}

var (
	faketime = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	handlerTests = []struct {
		Opts *tint.Options
		F    func(l *slog.Logger)
		Want string
	}{
		{
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `Nov 10 23:00:00.000 INF test key=val`,
		},
		{
			F: func(l *slog.Logger) {
				l.Error("test", tint.Err(errors.New("fail")))
			},
			Want: `Nov 10 23:00:00.000 ERR test err=fail`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", slog.Group("group", slog.String("key", "val"), tint.Err(errors.New("fail"))))
			},
			Want: `Nov 10 23:00:00.000 INF test group.key=val group.err=fail`,
		},
		{
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val")
			},
			Want: `Nov 10 23:00:00.000 INF test group.key=val`,
		},
		{
			F: func(l *slog.Logger) {
				l.With("key", "val").Info("test", "key2", "val2")
			},
			Want: `Nov 10 23:00:00.000 INF test key=val key2=val2`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "k e y", "v a l")
			},
			Want: `Nov 10 23:00:00.000 INF test "k e y"="v a l"`,
		},
		{
			F: func(l *slog.Logger) {
				l.WithGroup("g r o u p").Info("test", "key", "val")
			},
			Want: `Nov 10 23:00:00.000 INF test "g r o u p.key"=val`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "slice", []string{"a", "b", "c"}, "map", map[string]int{"a": 1, "b": 2, "c": 3})
			},
			Want: `Nov 10 23:00:00.000 INF test slice="[a b c]" map="map[a:1 b:2 c:3]"`,
		},
		{
			Opts: &tint.Options{
				AddSource: true,
				NoColor:   true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `Nov 10 23:00:00.000 INF tint/handler_test.go:134 test key=val`,
		},
		{
			Opts: &tint.Options{
				TimeFormat: time.Kitchen,
				NoColor:    true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `11:00PM INF test key=val`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: drop(slog.TimeKey),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `INF test key=val`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: drop(slog.LevelKey),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `Nov 10 23:00:00.000 test key=val`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: drop(slog.MessageKey),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `Nov 10 23:00:00.000 INF key=val`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: drop(slog.TimeKey, slog.LevelKey, slog.MessageKey),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `key=val`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: drop("key"),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `Nov 10 23:00:00.000 INF test`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: drop("key"),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val", "key2", "val2")
			},
			Want: `Nov 10 23:00:00.000 INF test group.key=val group.key2=val2`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == "key" && len(groups) == 1 && groups[0] == "group" {
						return slog.Attr{}
					}
					return a
				},
				NoColor: true,
			},
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val", "key2", "val2")
			},
			Want: `Nov 10 23:00:00.000 INF test group.key2=val2`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: replace(slog.IntValue(42), slog.TimeKey),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `42 INF test key=val`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: replace(slog.StringValue("INFO"), slog.LevelKey),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `Nov 10 23:00:00.000 INFO test key=val`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: replace(slog.IntValue(42), slog.MessageKey),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `Nov 10 23:00:00.000 INF 42 key=val`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: replace(slog.IntValue(42), "key"),
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.With("key", "val").Info("test", "key2", "val2")
			},
			Want: `Nov 10 23:00:00.000 INF test key=42 key2=val2`,
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					return slog.Attr{}
				},
				NoColor: true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: ``,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "key", "")
			},
			Want: `Nov 10 23:00:00.000 INF test key=""`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "", "val")
			},
			Want: `Nov 10 23:00:00.000 INF test ""=val`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "", "")
			},
			Want: `Nov 10 23:00:00.000 INF test ""=""`,
		},
		{
			Opts: &tint.Options{
				TimeFormat: time.DateOnly,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if len(groups) == 0 && a.Key == slog.TimeKey {
						return slog.Time(slog.TimeKey, a.Value.Time().Add(24*time.Hour))
					}
					return a
				},
				NoColor: true,
			},
			F: func(l *slog.Logger) {
				l.Info("test")
			},
			Want: `2009-11-11 INF test`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "lvl", slog.LevelWarn)
			},
			Want: `Nov 10 23:00:00.000 INF test lvl=WARN`,
		},
		{
			Opts: &tint.Options{NoColor: false},
			F: func(l *slog.Logger) {
				l.Info("test", "lvl", slog.LevelWarn)
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m test \033[2mlvl=\033[0mWARN",
		},
		{
			Opts: &tint.Options{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					return tint.Attr(13, a)
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test")
			},
			Want: "\033[2;95mNov 10 23:00:00.000\033[0m \033[95mINF\033[0m \033[95mtest\033[0m",
		},
		{
			Opts: &tint.Options{NoColor: false},
			F: func(l *slog.Logger) {
				l.Error("test", tint.Err(errors.New("fail")))
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[91mERR\033[0m test \033[2;91merr=\033[22mfail\033[0m",
		},
		{
			Opts: &tint.Options{NoColor: false},
			F: func(l *slog.Logger) {
				l.Info("test", tint.Attr(10, slog.String("key", "value")))
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m test \033[2;92mkey=\033[22mvalue\033[0m",
		},
		{
			Opts: &tint.Options{NoColor: false},
			F: func(l *slog.Logger) {
				l.Info("test", tint.Attr(226, slog.String("key", "value")))
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m test \033[2;38;5;226mkey=\033[22mvalue\033[0m",
		},
		{
			Opts: &tint.Options{
				NoColor: false,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.MessageKey && len(groups) == 0 {
						return tint.Attr(10, a)
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "value")
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m \033[92mtest\033[0m \033[2mkey=\033[0mvalue",
		},
		{
			Opts: &tint.Options{
				NoColor: false,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey && len(groups) == 0 {
						return tint.Attr(10, a)
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "value")
			},
			Want: "\033[2;92mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m test \033[2mkey=\033[0mvalue",
		},
		{
			Opts: &tint.Options{
				NoColor: false,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey && len(groups) == 0 {
						return tint.Attr(10, slog.String(a.Key, a.Value.Time().Format(time.StampMilli)))
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "value")
			},
			Want: "\033[2;92mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m test \033[2mkey=\033[0mvalue",
		},
		{
			Opts: &tint.Options{
				AddSource: true,
				NoColor:   false,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.SourceKey && len(groups) == 0 {
						return tint.Attr(10, a)
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test")
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m \033[2;92mtint/handler_test.go:411\033[0m test",
		},
		{
			Opts: &tint.Options{
				NoColor: false,
				Level:   slog.LevelDebug - 4,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.LevelKey && len(groups) == 0 {
						level, ok := a.Value.Any().(slog.Level)
						if ok && level <= slog.LevelDebug-4 {
							return slog.String(a.Key, "TRC")
						}
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				const levelTrace = slog.LevelDebug - 4
				l.Log(context.TODO(), levelTrace, "test")
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m TRC test",
		},
		{
			Opts: &tint.Options{
				NoColor: false,
				Level:   slog.LevelDebug - 4,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.LevelKey && len(groups) == 0 {
						level, ok := a.Value.Any().(slog.Level)
						if ok && level <= slog.LevelDebug-4 {
							return tint.Attr(13, slog.String(a.Key, "TRC"))
						}
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				const levelTrace = slog.LevelDebug - 4
				l.Log(context.TODO(), levelTrace, "test")
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[95mTRC\033[0m test",
		},

		{ // https://github.com/lmittmann/tint/issues/8
			F: func(l *slog.Logger) {
				l.Log(context.TODO(), slog.LevelInfo+1, "test")
			},
			Want: `Nov 10 23:00:00.000 INF+1 test`,
		},
		{
			Opts: &tint.Options{
				Level:   slog.LevelDebug - 1,
				NoColor: true,
			},
			F: func(l *slog.Logger) {
				l.Log(context.TODO(), slog.LevelDebug-1, "test")
			},
			Want: `Nov 10 23:00:00.000 DBG-1 test`,
		},
		{ // https://github.com/lmittmann/tint/issues/12
			F: func(l *slog.Logger) {
				l.Error("test", slog.Any("error", errors.New("fail")))
			},
			Want: `Nov 10 23:00:00.000 ERR test error=fail`,
		},
		{ // https://github.com/lmittmann/tint/issues/15
			F: func(l *slog.Logger) {
				l.Error("test", tint.Err(nil))
			},
			Want: `Nov 10 23:00:00.000 ERR test err=<nil>`,
		},
		{ // https://github.com/lmittmann/tint/pull/26
			Opts: &tint.Options{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey && len(groups) == 0 {
						return slog.Time(slog.TimeKey, a.Value.Time().Add(24*time.Hour))
					}
					return a
				},
				NoColor: true,
			},
			F: func(l *slog.Logger) {
				l.Error("test")
			},
			Want: `Nov 11 23:00:00.000 ERR test`,
		},
		{ // https://github.com/lmittmann/tint/pull/27
			F: func(l *slog.Logger) {
				l.Info("test", "a", "b", slog.Group("", slog.String("c", "d")), "e", "f")
			},
			Want: `Nov 10 23:00:00.000 INF test a=b c=d e=f`,
		},
		{ // https://github.com/lmittmann/tint/pull/30
			// drop built-in attributes in a grouped log
			Opts: &tint.Options{
				ReplaceAttr: drop(slog.TimeKey, slog.LevelKey, slog.MessageKey, slog.SourceKey),
				AddSource:   true,
				NoColor:     true,
			},
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val")
			},
			Want: `group.key=val`,
		},
		{ // https://github.com/lmittmann/tint/issues/36
			Opts: &tint.Options{
				ReplaceAttr: func(g []string, a slog.Attr) slog.Attr {
					if len(g) == 0 && a.Key == slog.LevelKey {
						_ = a.Value.Any().(slog.Level)
					}
					return a
				},
				NoColor: true,
			},
			F: func(l *slog.Logger) {
				l.Info("test")
			},
			Want: `Nov 10 23:00:00.000 INF test`,
		},
		{ // https://github.com/lmittmann/tint/issues/37
			Opts: &tint.Options{
				AddSource: true,
				ReplaceAttr: func(g []string, a slog.Attr) slog.Attr {
					return a
				},
				NoColor: true,
			},
			F: func(l *slog.Logger) {
				l.Info("test")
			},
			Want: `Nov 10 23:00:00.000 INF tint/handler_test.go:541 test`,
		},
		{ // https://github.com/lmittmann/tint/issues/44
			F: func(l *slog.Logger) {
				l = l.WithGroup("group")
				l.Error("test", tint.Err(errTest))
			},
			Want: `Nov 10 23:00:00.000 ERR test group.err=fail`,
		},
		{ // https://github.com/lmittmann/tint/issues/55
			F: func(l *slog.Logger) {
				l.Info("test", "key", struct {
					A int
					B *string
				}{A: 123})
			},
			Want: `Nov 10 23:00:00.000 INF test key="{A:123 B:<nil>}"`,
		},
		{ // https://github.com/lmittmann/tint/issues/59
			Opts: &tint.Options{NoColor: false},
			F: func(l *slog.Logger) {
				l.Info("test", "color", "\033[92mgreen\033[0m")
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m test \033[2mcolor=\033[0m\033[92mgreen\033[0m",
		},
		{
			Opts: &tint.Options{NoColor: false},
			F: func(l *slog.Logger) {
				l.Info("test", "color", "\033[92mgreen quoted\033[0m")
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m test \033[2mcolor=\033[0m\"\033[92mgreen quoted\033[0m\"",
		},
		{
			Opts: &tint.Options{NoColor: true},
			F: func(l *slog.Logger) {
				l.Info("test", "color", "\033[92mgreen\033[0m")
			},
			Want: `Nov 10 23:00:00.000 INF test color=green`,
		},
		{
			Opts: &tint.Options{NoColor: true},
			F: func(l *slog.Logger) {
				l.Info("test", "color", "\033[92mgreen quoted\033[0m")
			},
			Want: `Nov 10 23:00:00.000 INF test color="green quoted"`,
		},
		{ // https://github.com/lmittmann/tint/pull/66
			F: func(l *slog.Logger) {
				errAttr := tint.Err(errors.New("fail"))
				errAttr.Key = "error"
				l.Error("test", errAttr)
			},
			Want: `Nov 10 23:00:00.000 ERR test error=fail`,
		},
		{ // https://github.com/lmittmann/tint/issues/85
			F: func(l *slog.Logger) {
				var t *time.Time
				l.Info("test", "time", t)
			},
			Want: `Nov 10 23:00:00.000 INF test time=<nil>`,
		},
		{ // https://github.com/lmittmann/tint/pull/94
			F: func(l *slog.Logger) {
				l.Info("test", "time", testTime)
			},
			Want: `Nov 10 23:00:00.000 INF test time=2022-05-01T00:00:00.000Z`,
		},
		{ // https://github.com/lmittmann/tint/pull/96
			Opts: &tint.Options{
				NoColor: false,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test", tint.Attr(10, slog.String("key", "val")))
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[92mINF\033[0m test \033[2;92mkey=\033[22mval\033[0m",
		},
		{
			Opts: &tint.Options{
				NoColor: false,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					return tint.Attr(13, a)
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test", tint.Attr(10, slog.String("key", "val")))
			},
			Want: "\033[2;95mNov 10 23:00:00.000\033[0m \033[95mINF\033[0m \033[95mtest\033[0m \033[2;95mkey=\033[22mval\033[0m",
		},
		{ // https://github.com/lmittmann/tint/issues/100
			Opts: &tint.Options{
				NoColor:   false,
				Level:     slog.LevelDebug,
				AddSource: true,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.LevelKey && len(groups) == 0 {
						if _, ok := a.Value.Any().(slog.Level); ok {
							return tint.Attr(13, a)
						}
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Debug("test")
			},
			Want: "\033[2mNov 10 23:00:00.000\033[0m \033[95mDBG\033[0m \033[2mtint/handler_test.go:649\033[0m test",
		},
		{ // https://github.com/lmittmann/tint/pr/
			Opts: &tint.Options{NoColor: true},
			F: func(l *slog.Logger) {
				l.Info("test", "key", json.RawMessage(`{"k":"v"}`))
			},
			Want: `Nov 10 23:00:00.000 INF test key="{\"k\":\"v\"}"`,
		},
	}
)

func TestHandler(t *testing.T) {
	if now := time.Now(); !faketime.Equal(now) || now.Location().String() != "UTC" {
		t.Skip(`run: TZ="" go test -tags=faketime`)
	}

	for i, test := range handlerTests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var buf bytes.Buffer
			if test.Opts == nil {
				test.Opts = &tint.Options{NoColor: true}
			}
			l := slog.New(tint.NewHandler(&buf, test.Opts))
			test.F(l)

			got, foundNewline := strings.CutSuffix(buf.String(), "\n")
			if !foundNewline {
				t.Fatalf("missing newline")
			}
			if test.Want != got {
				t.Fatalf("(-want +got)\n- %s\n+ %s", test.Want, got)
			}
		})
	}
}

// drop returns a ReplaceAttr that drops the given keys.
func drop(keys ...string) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) > 0 {
			return a
		}

		for _, key := range keys {
			if a.Key == key {
				a = slog.Attr{}
			}
		}
		return a
	}
}

func replace(new slog.Value, keys ...string) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) > 0 {
			return a
		}

		for _, key := range keys {
			if a.Key == key {
				a.Value = new
			}
		}
		return a
	}
}

func TestHandler_Consistency(t *testing.T) {
	if now := time.Now(); !faketime.Equal(now) || now.Location().String() != "UTC" {
		t.Skip(`run: TZ="" go test -tags=faketime`)
	}

	tests := []struct {
		Val any
	}{
		{"val"},
		{123},
		{123.456},
		{true},
		{false},
		{nil},
		{time.Now()},
		{time.Now().In(time.UTC)},
		{time.Duration(123456789 * time.Second)},
		{errors.New("error")},
		{tint.Err(errors.New("error"))},
		{[]string{"a", "b", "c"}},
		{map[string]int{"a": 1, "b": 2, "c": 3}},
		{[]byte{0xc0, 0xfe}},
		{[]byte("hello")},
		{json.RawMessage(`{"k":"v"}`)},
	}

	// drop all attributes except "key"
	rep := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key != "key" {
			return slog.Attr{}
		}
		return a
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			// log with tint.Handler
			var tintBuf bytes.Buffer
			tintLogger := slog.New(tint.NewHandler(&tintBuf, &tint.Options{
				NoColor:     true,
				ReplaceAttr: rep,
			}))
			tintLogger.Info("test", "key", test.Val)

			// log with slog.TextHandler
			var textBuf bytes.Buffer
			textLogger := slog.New(slog.NewTextHandler(&textBuf, &slog.HandlerOptions{
				ReplaceAttr: rep,
			}))
			textLogger.Info("test", "key", test.Val)

			if textBuf.String() != tintBuf.String() {
				t.Fatalf("(-want +got)\n- %s\n+ %s", textBuf.String(), tintBuf.String())
			}
		})
	}
}

func TestReplaceAttr(t *testing.T) {
	if now := time.Now(); !faketime.Equal(now) || now.Location().String() != "UTC" {
		t.Skip(`run: TZ="" go test -tags=faketime`)
	}

	tests := [][]any{
		{},
		{"key", "val"},
		{"key", "val", slog.Group("group", "key2", "val2")},
		{"key", "val", slog.Group("group", "key2", "val2", slog.Group("group2", "key3", "val3"))},
	}

	type replaceAttrParams struct {
		Groups []string
		Attr   slog.Attr
	}

	replaceAttrRecorder := func(record *[]replaceAttrParams) func([]string, slog.Attr) slog.Attr {
		return func(groups []string, a slog.Attr) slog.Attr {
			*record = append(*record, replaceAttrParams{groups, a})
			return a
		}
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			slogRecord := make([]replaceAttrParams, 0)
			slogLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
				ReplaceAttr: replaceAttrRecorder(&slogRecord),
			}))
			slogLogger.Log(context.TODO(), slog.LevelInfo, "", test...)

			tintRecord := make([]replaceAttrParams, 0)
			tintLogger := slog.New(tint.NewHandler(io.Discard, &tint.Options{
				ReplaceAttr: replaceAttrRecorder(&tintRecord),
			}))
			tintLogger.Log(context.TODO(), slog.LevelInfo, "", test...)

			if !slices.EqualFunc(slogRecord, tintRecord, func(a, b replaceAttrParams) bool {
				return slices.Equal(a.Groups, b.Groups) && a.Attr.Equal(b.Attr)
			}) {
				t.Fatalf("(-want +got)\n- %v\n+ %v", slogRecord, tintRecord)
			}
		})
	}
}

func TestAttr(t *testing.T) {
	if now := time.Now(); !faketime.Equal(now) || now.Location().String() != "UTC" {
		t.Skip(`run: TZ="" go test -tags=faketime`)
	}

	t.Run("text", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		logger.Info("test", tint.Attr(10, slog.String("key", "val")))

		want := `time=2009-11-10T23:00:00.000Z level=INFO msg=test key=val`
		if got := strings.TrimSpace(buf.String()); want != got {
			t.Fatalf("(-want +got)\n- %s\n+ %s", want, got)
		}
	})

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		logger.Info("test", tint.Attr(10, slog.String("key", "val")))

		want := `{"time":"2009-11-10T23:00:00Z","level":"INFO","msg":"test","key":"val"}`
		if got := strings.TrimSpace(buf.String()); want != got {
			t.Fatalf("(-want +got)\n- %s\n+ %s", want, got)
		}
	})
}

// TestClonedHandlersSynchronizeWriter tests that cloned handlers synchronize writer
// writes with each other such that a logger can be shared among multiple goroutines.
func TestClonedHandlersSynchronizeWriter(t *testing.T) {
	// logSomething calls `With(...)` and uses the resulting logger to create and use a cloned handler.
	logSomething := func(wg *sync.WaitGroup, logger *slog.Logger, loggerID int) {
		defer wg.Done()
		logger = logger.With("withLoggerID", loggerID)
		logger.Info("test")
	}

	logger := slog.New(tint.NewHandler(&bytes.Buffer{}, &tint.Options{}))

	// start and wait for two goroutines
	var wg sync.WaitGroup
	wg.Add(2)
	go logSomething(&wg, logger, 1)
	go logSomething(&wg, logger, 2)
	wg.Wait()
}

// See https://github.com/golang/exp/blob/master/slog/benchmarks/benchmarks_test.go#L25
//
// Run e.g.:
//
//	go test -bench=. -count=10 | benchstat -col /h /dev/stdin
func BenchmarkLogAttrs(b *testing.B) {
	handler := []struct {
		Name string
		H    slog.Handler
	}{
		{"tint", tint.NewHandler(io.Discard, nil)},
		{"text", slog.NewTextHandler(io.Discard, nil)},
		{"json", slog.NewJSONHandler(io.Discard, nil)},
		{"discard", new(discarder)},
	}

	benchmarks := []struct {
		Name string
		F    func(*slog.Logger)
	}{
		{
			"5 args",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelInfo, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
				)
			},
		},
		{
			"5 args custom level",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelInfo+1, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
				)
			},
		},
		{
			"10 args",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelInfo, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
				)
			},
		},
		{
			"40 args",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelInfo, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
				)
			},
		},
		{
			"error",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelError, testMessage,
					tint.Err(errTest),
				)
			},
		},
		{
			"attr",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelError, testMessage,
					tint.Attr(9, slog.String("string", testString)),
				)
			},
		},
	}

	for _, h := range handler {
		b.Run("h="+h.Name, func(b *testing.B) {
			for _, bench := range benchmarks {
				b.Run(bench.Name, func(b *testing.B) {
					b.ReportAllocs()
					logger := slog.New(h.H)
					for i := 0; i < b.N; i++ {
						bench.F(logger)
					}
				})
			}
		})
	}
}

// discarder is a slog.Handler that discards all records.
type discarder struct{}

func (*discarder) Enabled(context.Context, slog.Level) bool   { return true }
func (*discarder) Handle(context.Context, slog.Record) error  { return nil }
func (d *discarder) WithAttrs(attrs []slog.Attr) slog.Handler { return d }
func (d *discarder) WithGroup(name string) slog.Handler       { return d }

var (
	testMessage  = "Test logging, but use a somewhat realistic message length."
	testTime     = time.Date(2022, time.May, 1, 0, 0, 0, 0, time.UTC)
	testString   = "7e3b3b2aaeff56a7108fe11e154200dd/7819479873059528190"
	testInt      = 32768
	testDuration = 23 * time.Second
	errTest      = errors.New("fail")
)
