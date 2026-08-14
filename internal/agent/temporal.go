package agent

import (
	"fmt"
	"strings"
	"time"
)

// temporalAnchor returns a fresh per-turn clock note for the model.
// Appended as a footer after the user message (not a header before it) so the
// model reads intent first; not persisted in session history — regenerated on
// every Handle. NOW and the day-part are first so meal/calendar planning does
// not slide to lunch or a future event. The week grid uses ISO dates so
// weekday-only memory ("Monday") cannot be reused after the week rolls.
func temporalAnchor(now time.Time, tzName string) string {
	if tzName == "" {
		tzName = now.Location().String()
	}
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)
	// "-07:00" is Go's layout token (like "2006-01-02"), not a fixed Pacific
	// offset — it prints whatever zone `now` is in (from CRON_TZ / Location).
	offset := now.Format("-07:00")
	part, already, ahead := dayPart(now.Hour())

	return fmt.Sprintf(
		"[current time] NOW: %s %s %d, %d %s (%s, UTC%s) — %s\n"+
			"already today: 12:00 AM–%s (%s)\n"+
			"remaining today: %s–11:59 PM (%s)\n"+
			"yesterday=%s (%s) · today=%s (%s) · tomorrow=%s (%s)\n"+
			"this week: %s\n"+
			"next week starts %s — weekday-only notes (e.g. \"Monday\") are not this week\n"+
			"Relative words in earlier turns, summary, and memory are stale. NOW is the split: before = past, after = upcoming. "+
			"Tool date args: this timezone, offset %s — not UTC/Z.",
		now.Weekday().String(),
		now.Month().String(),
		now.Day(),
		now.Year(),
		now.Format("3:04 PM"),
		tzName,
		offset,
		dayPartCallout(part),
		now.Format("3:04 PM"),
		joinParts(already),
		now.Format("3:04 PM"),
		joinParts(ahead),
		yesterday.Format("2006-01-02"),
		yesterday.Weekday().String(),
		now.Format("2006-01-02"),
		now.Weekday().String(),
		tomorrow.Format("2006-01-02"),
		tomorrow.Weekday().String(),
		formatWeek(now),
		sundayStart(now).AddDate(0, 0, 7).Format("2006-01-02"),
		offset,
	)
}

type daySlot struct {
	name string
	lo   int // inclusive hour
	hi   int // exclusive hour
}

var daySlots = []daySlot{
	{name: "overnight", lo: 0, hi: 5},
	{name: "early morning", lo: 5, hi: 8},
	{name: "morning", lo: 8, hi: 11},
	{name: "lunch", lo: 11, hi: 13},
	{name: "afternoon", lo: 13, hi: 17},
	{name: "evening", lo: 17, hi: 21},
	{name: "night", lo: 21, hi: 24},
}

func dayPart(hour int) (name string, already, ahead []string) {
	idx := 0
	for i, s := range daySlots {
		if hour >= s.lo && hour < s.hi {
			idx = i
			name = s.name
			break
		}
	}
	for i, s := range daySlots {
		switch {
		case i < idx:
			already = append(already, s.name)
		case i > idx:
			ahead = append(ahead, s.name)
		}
	}
	return name, already, ahead
}

func dayPartCallout(name string) string {
	switch name {
	case "lunch":
		return "lunch window"
	case "afternoon", "evening", "night":
		return name + " (lunch is over)"
	default:
		return name + ", not lunch"
	}
}

func joinParts(parts []string) string {
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func sundayStart(now time.Time) time.Time {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return midnight.AddDate(0, 0, -int(now.Weekday()))
}

func formatWeek(now time.Time) string {
	start := sundayStart(now)
	parts := make([]string, 7)
	for i := 0; i < 7; i++ {
		d := start.AddDate(0, 0, i)
		parts[i] = fmt.Sprintf("%s %s", d.Format("Mon 01-02"), dayMark(d, now))
	}
	return strings.Join(parts, " · ")
}

func dayMark(d, now time.Time) string {
	dy, dm, dd := d.Date()
	ny, nm, nd := now.Date()
	if dy == ny && dm == nm && dd == nd {
		return "TODAY"
	}
	if time.Date(dy, dm, dd, 0, 0, 0, 0, time.UTC).Before(time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC)) {
		return "past"
	}
	return "upcoming"
}
