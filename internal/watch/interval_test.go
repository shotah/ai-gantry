package watch_test

import (
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/watch"
)

func TestParseInterval(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", watch.DefaultInterval, false},
		{"15m", 15 * time.Minute, false},
		{"1h", time.Hour, false},
		{"900", 15 * time.Minute, false},
		{"30s", 0, true},
		{"0", 0, true},
		{"nope", 0, true},
		{"240h", 0, true},
	}
	for _, tc := range cases {
		got, err := watch.ParseInterval(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %v err=%v want %v", tc.in, got, err, tc.want)
		}
	}
}
