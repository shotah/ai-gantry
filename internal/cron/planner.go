package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// KindDailyPlanner is the once-a-day planning session.
const KindDailyPlanner = "daily_planner"

// DefaultPlannerAt is the local clock when DAILY_PLANNER_AT is unset.
const DefaultPlannerAt = "07:10"

// PlannerOff opts the session out of the daily planner.
const PlannerOff = "0"

// DailyPlannerMarker is the start of DailyPlannerPrefix. The agent uses it
// to require live tools on this turn.
const DailyPlannerMarker = "[cron] Daily planner"

// DailyPlannerPrefix wraps the once-a-day planning session.
const DailyPlannerPrefix = DailyPlannerMarker + " — one planning session for the day. [hours], [aims], [loops], and [wakes] are already in this turn's [harness]; do not memory_recall or cron_list for those. In one response: review [mcp prefixes], mcp_enable what is off, then pull calendar, mail, and Garmin (or the fitness server that is on). Do not invent numbers or events.\n\n"

// DefaultDailyPlannerPrompt is the kernel-owned body of the daily session.
// The model does not replace it; cron_schedule repeat=planner only moves the clock.
const DefaultDailyPlannerPrompt = "" +
	"Plan today from the tool results.\n" +
	"cron_schedule the day's cues (leave, prep, a check they asked for). A calendar event is not the chat reminder.\n" +
	"Ask first before creating or moving calendar events, sending mail, spending, or posting.\n" +
	"Sick, on vacation, a holiday they have off, or they asked for a quiet day: reply [silent], do not add nag crons, and cron_cancel day-plan wakes that no longer fit. Do not schedule extra sessions to re-check.\n" +
	"If this clock does not match their life (student, night shift, still asleep at this hour), move it: cron_schedule when=HH:MM repeat=planner. That persists. Do not schedule a second planner.\n" +
	"No [aims] line at all: ask ONE months-scale question (do not invent), [wait], memory_store fact subject aim/bootstrap that you asked. `[aims] none (asked …)` means already asked — do not ask again.\n" +
	"[hours] unknown: ask sleep / work / quiet once (work is not DND), [wait], memory_store preference subject pref/hours.\n" +
	"A real empty calendar is a hole, not [silent]: ask ONE what they want on it today (lunch/dinner or training). [wait]. Try to get something scheduled. Never agree-and-stop.\n" +
	"If [room] is stamped and stale for the hour, redress it, then [silent].\n" +
	"After the pulls, aim_log yesterday: each tool event with its ref, scored against every aim it touches, in one call. 0 for a planned rest or a weigh-in. On a quit aim a clean day is +1. what quotes the tool or them.\n" +
	"Day cues for an aim carry memory_subject aim/<area>.\n" +
	"When [progress] carries weeks: it is the week's first session. One line per aim from those numbers (the week means, the slope, the block fraction, the effect line if present) before the ladder, then the ladder as usual; that reply is not [silent]. No weeks: line means an ordinary morning; do not summarize the week. A missing r= is not a correlation and not a guess; say nothing about it. On pace stays [silent].\n" +
	"Read [progress]. An empty today (·) does not erase the run behind it. First matching rung wins, and that rung is the reply — do not [silent] past it:\n" +
	"streak ≥3 and no praised this week → one credit line, aim_log note=praised.\n" +
	"yesterday <0 and the last note is already nudged, or two against-days → ask what is in the way, [wait], aim_log note=asked. Not another nudge, not [silent].\n" +
	"yesterday <0 and the last note is not nudged → one nudge tied to the tool, note=nudged.\n" +
	"three against-days or a -3 → offer a smaller plan, note=offered.\n" +
	"today's calendar or mail works against an aim (a dinner out while losing weight, a skipped session the tool showed) → one line about that fact, aim_log it, not [silent]. A weigh-in copies the tool's number onto metric and value and scores 0 — on pace is [silent], not a +3.\n" +
	"yesterday >0 and none of the rungs above → [silent] on that aim.\n" +
	"Never repeat the last note. A slip they already owned → score it, note=quiet, no lecture.\n" +
	"A tool is on and the aim has no event in 3 days → aim_log from the tool; do not ask for the number.\n" +
	"When the plan has to change, aim_history first and talk from those rows.\n" +
	"[silent] only when the rung above says so. A credit, an ask, or a meal thought is the reply. A question they should answer → [wait] on its own line.\n" +
	"When weeks: is in [progress], those lines come first: one line per aim (direction, slope, block, effect if present), and that reply is not [silent]. No weeks: line: do not summarize the week. A missing r= is not a correlation."

// IsDailyPlannerTurn reports whether this user text is the daily planning session.
func IsDailyPlannerTurn(userText string) bool {
	return strings.HasPrefix(strings.TrimSpace(userText), DailyPlannerMarker)
}

// ParsePlannerAt accepts "07:10", "7:10", "9am", "9:30pm".
func ParsePlannerAt(s string) (hour, minute int, err error) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "planner:")
	s = strings.TrimSpace(s)
	if s == "" || s == PlannerOff {
		return 0, 0, fmt.Errorf("cron: empty planner time")
	}
	ampm := ""
	switch {
	case strings.HasSuffix(s, "am"), strings.HasSuffix(s, "pm"):
		ampm = s[len(s)-2:]
		s = strings.TrimSpace(s[:len(s)-2])
	}
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		if len(parts) != 2 {
			return 0, 0, fmt.Errorf("cron: bad planner time %q", s)
		}
		hour, err = strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, 0, fmt.Errorf("cron: bad planner time %q", s)
		}
		minute, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("cron: bad planner time %q", s)
		}
	} else {
		hour, err = strconv.Atoi(s)
		if err != nil {
			return 0, 0, fmt.Errorf("cron: bad planner time %q", s)
		}
	}
	if ampm == "pm" && hour < 12 {
		hour += 12
	}
	if ampm == "am" && hour == 12 {
		hour = 0
	}
	if ampm != "" && (hour < 0 || hour > 23) {
		return 0, 0, fmt.Errorf("cron: bad planner time")
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("cron: planner time must be a clock (got %02d:%02d)", hour, minute)
	}
	return hour, minute, nil
}

// FormatPlannerAt renders HH:MM.
func FormatPlannerAt(hour, minute int) string {
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

// NextPlannerAt is the next fire of the daily session.
// A session already running, or one that already ran today, waits until tomorrow
// so moving the clock mid-session does not buy a second burn today.
func NextPlannerAt(hour, minute int, loc *time.Location, now time.Time, lastRun *time.Time, running bool) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)
	slot := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if running || (lastRun != nil && sameLocalDay(lastRun.In(loc), now)) {
		return addOneCalendarDay(slot).UTC()
	}
	if slot.After(now) {
		return slot.UTC()
	}
	return addOneCalendarDay(slot).UTC()
}

func sameLocalDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// ParsePlannerSchedule builds a KindDailyPlanner job at the next clock time.
func ParsePlannerSchedule(when string, loc *time.Location, now time.Time) (Parsed, error) {
	hour, minute, err := ParsePlannerAt(when)
	if err != nil {
		return Parsed{}, err
	}
	if loc == nil {
		loc = time.UTC
	}
	return Parsed{
		Kind:     KindDailyPlanner,
		Expr:     FormatPlannerAt(hour, minute),
		NextRun:  NextPlannerAt(hour, minute, loc, now, nil, false),
		Timezone: loc.String(),
	}, nil
}
