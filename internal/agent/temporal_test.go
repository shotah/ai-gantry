package agent

import (
	"strings"
	"testing"
	"time"
)

func TestTemporalAnchor(t *testing.T) {
	loc := time.FixedZone("America/Los_Angeles", -7*3600)
	now := time.Date(2026, time.July, 26, 8, 3, 0, 0, loc)
	got := temporalAnchor(now, "America/Los_Angeles")
	for _, want := range []string{
		"[current time]",
		now.Weekday().String(),
		"July",
		"26",
		"2026",
		"8:03 AM",
		"America/Los_Angeles",
		"UTC-07:00",
		"yesterday=2026-07-25 (Saturday)",
		"today=2026-07-26",
		"tomorrow=2026-07-27 (Monday)",
		"do not default to UTC/Z",
		"offset -07:00",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("got %q, want substring %q", got, want)
		}
	}
}
