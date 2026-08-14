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
		"[current time] NOW:",
		now.Weekday().String(),
		"July",
		"26",
		"2026",
		"8:03 AM",
		"America/Los_Angeles",
		"offset -07:00",
		"Human local time, not UTC",
		"morning, not lunch",
		"already today: 12:00 AM–8:03 AM (overnight, early morning)",
		"remaining today: 8:03 AM–11:59 PM (lunch, afternoon, evening, night)",
		"yesterday=2026-07-25 (Saturday)",
		"today=2026-07-26 (Sunday)",
		"tomorrow=2026-07-27 (Monday)",
		"Sun 07-26 TODAY",
		"Mon 07-27 upcoming",
		"next week starts 2026-08-02",
		"weekday-only notes",
		"never Z / UTC",
		"offset -07:00",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("got %q, want substring %q", got, want)
		}
	}
}

func TestTemporalAnchor_MorningNotLunch(t *testing.T) {
	loc := time.FixedZone("America/Los_Angeles", -7*3600)
	now := time.Date(2026, time.August, 14, 9, 3, 0, 0, loc)
	got := temporalAnchor(now, "America/Los_Angeles")
	for _, want := range []string{
		"[current time] NOW:",
		"9:03 AM",
		"morning, not lunch",
		"remaining today: 9:03 AM–11:59 PM (lunch, afternoon, evening, night)",
		"today=2026-08-14 (Friday)",
		"Fri 08-14 TODAY",
		"Sat 08-15 upcoming",
		"Sun 08-09 past",
		"next week starts 2026-08-16",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("got %q, want substring %q", got, want)
		}
	}
	if strings.Contains(got, "lunch window") {
		t.Fatalf("9am must not claim lunch: %q", got)
	}
	if strings.Contains(got, "UTC-") || strings.Contains(got, "UTC+") {
		t.Fatalf("Pacific clock must not say UTC±: %q", got)
	}
}

func TestTemporalAnchor_PacificDoesNotSayUTC(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip(err)
	}
	now := time.Date(2026, time.August, 14, 9, 3, 0, 0, loc)
	got := temporalAnchor(now, "America/Los_Angeles")
	if strings.Contains(got, "UTC-") || strings.Contains(got, "UTC+") || strings.Contains(got, ", UTC") {
		t.Fatalf("must not label Pacific as UTC: %q", got)
	}
	if !strings.Contains(got, "Human local time, not UTC") {
		t.Fatalf("got %q, want explicit not-UTC", got)
	}
	if !strings.Contains(got, "America/Los_Angeles") {
		t.Fatalf("got %q, want IANA zone", got)
	}
}

func TestTemporalAnchor_LunchWindow(t *testing.T) {
	loc := time.FixedZone("America/Los_Angeles", -7*3600)
	now := time.Date(2026, time.August, 14, 12, 15, 0, 0, loc)
	got := temporalAnchor(now, "America/Los_Angeles")
	if !strings.Contains(got, "lunch window") {
		t.Fatalf("got %q, want lunch window", got)
	}
	if !strings.Contains(got, "already today: 12:00 AM–12:15 PM (overnight, early morning, morning)") {
		t.Fatalf("got %q, want morning already elapsed", got)
	}
}
