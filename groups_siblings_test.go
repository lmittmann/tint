package tint_test

import (
	"bytes"
	"log/slog"
	"slices"
	"testing"

	"github.com/lmittmann/tint"
)

func TestWithGroupSiblingsDoNotShareState(t *testing.T) {
	var buf bytes.Buffer
	var got []string

	h := tint.NewTextHandler(&buf, &tint.Options{
		NoColor: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "k" {
				got = append([]string(nil), groups...)
			}
			return a
		},
	})

	base := slog.New(h.WithGroup("a").WithGroup("b").WithGroup("c"))
	l1 := base.WithGroup("x")
	_ = base.WithGroup("y")

	l1.Info("msg", "k", 1)

	want := []string{"a", "b", "c", "x"}
	if !slices.Equal(got, want) {
		t.Fatalf("ReplaceAttr groups = %q, want %q", got, want)
	}
}
