package tint_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/lmittmann/tint"
)

func TestNoColorPreservesStringBytes(t *testing.T) {
	cases := []struct {
		name, key, value, want string
	}{
		{"replacement_start", "key", "\ufffdtail", `key="�tail"`},
		{"replacement_middle", "key", "before\ufffdafter", `key="before�after"`},
		{"replacement_end", "key", "before\ufffd", `key="before�"`},
		{"invalid_start", "key", "\xfftail", `key="\xfftail"`},
		{"invalid_middle", "key", "before\xffafter", `key="before\xffafter"`},
		{"incomplete_utf8", "key", "before\xe2\x82after", `key="before\xe2\x82after"`},
		{"colored_replacement", "key", "\033[31mbefore\ufffdafter\033[0m", `key="before�after"`},
		{"replacement_key", "before\ufffdafter", "value", `"before�after"=value`},
		{"plain", "key", "value", `key=value`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := tint.NewTextHandler(&buf, &tint.Options{NoColor: true})
			r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
			r.AddAttrs(slog.String(tc.key, tc.value))
			if err := h.Handle(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			if got, want := buf.String(), "INF test "+tc.want+"\n"; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}
