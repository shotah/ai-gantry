package mcpenable

import (
	"strings"
	"testing"
	"time"
)

func TestFormatIndex_SortedStable(t *testing.T) {
	rows := []Row{
		{Prefix: "strava", Hold: HoldShort},
		{Prefix: "google__calendar", Hold: HoldLong},
	}
	index := []string{"flights", "google", "google__calendar", "google__gmail", "strava"}
	force := Force{Prefixes: []string{"garmin__sleep"}}
	a := FormatIndex(rows, index, force)
	b := FormatIndex(rows, index, force)
	if a != b {
		t.Fatal("FormatIndex must be byte-stable")
	}
	if strings.Contains(a, "ago") {
		t.Fatalf("prompt index must not include ages: %q", a)
	}
	on := strings.Index(a, "on:")
	off := strings.Index(a, "off:")
	if on < 0 || off < 0 {
		t.Fatalf("missing on/off: %q", a)
	}
	// keys in on: garmin, google__calendar, strava — sorted
	g := strings.Index(a, "garmin__sleep")
	c := strings.Index(a, "google__calendar")
	s := strings.Index(a, "strava")
	if g > c || c > s {
		t.Fatalf("on keys not sorted: %q", a)
	}
	// off walks sorted index: flights, google, google__gmail
	if i, j := strings.Index(a, "flights"), strings.Index(a, "google__gmail"); i < 0 || j < i {
		t.Fatalf("off keys not sorted: %q", a)
	}

	st := FormatIndexStatus(rows, index, force, time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(st, "ago") {
		t.Fatalf("/tools status should include ages: %q", st)
	}
}
