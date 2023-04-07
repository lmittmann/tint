package tint_test

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/lmittmann/tint"
	"golang.org/x/exp/slog"
)

func Example() {
	slog.SetDefault(slog.New(tint.Options{
		Level:      slog.LevelDebug,
		TimeFormat: time.Kitchen,
	}.NewHandler(os.Stderr)))

	slog.Info("Starting server", "addr", ":8080", "env", "production")
	slog.Debug("Connected to DB", "db", "myapp", "host", "localhost:5432")
	slog.Warn("Slow request", "method", "GET", "path", "/users", "duration", 497*time.Millisecond)
	slog.Error("DB connection lost", tint.Err(errors.New("connection reset")), "db", "myapp")
	// Output:
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
		{"tint", tint.NewHandler(io.Discard)},
		{"text", slog.NewTextHandler(io.Discard)},
		{"json", slog.NewJSONHandler(io.Discard)},
		{"discard", new(discarder)},
	}

	benchmarks := []struct {
		Name string
		F    func(*slog.Logger)
	}{
		{
			"5 args",
			func(logger *slog.Logger) {
				logger.LogAttrs(nil, slog.LevelInfo, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
				)
			},
		},
		{
			"5 args custom level",
			func(logger *slog.Logger) {
				logger.LogAttrs(nil, slog.LevelInfo+1, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
				)
			},
		},
		{
			"10 args",
			func(logger *slog.Logger) {
				logger.LogAttrs(nil, slog.LevelInfo, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
				)
			},
		},
		{
			"40 args",
			func(logger *slog.Logger) {
				logger.LogAttrs(nil, slog.LevelInfo, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", testError),
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
	testError    = errors.New("fail")
)
