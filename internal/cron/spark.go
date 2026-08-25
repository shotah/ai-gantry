package cron

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"
)

// KindSpark is the daily planner: rolls qty and inserts KindSparkPing jobs for the day.
const KindSpark = "spark"

// KindSparkPing is one horizon-planning wake created by the planner (one-shot).
const KindSparkPing = "spark_ping"

// DefaultSparkQty is how many looking-after-you wakes to seed per day.
// Chat /engagement | /spark overrides per session.
const DefaultSparkQty = "3-5"

// DefaultSparkStartHour is the local seed-window start (inclusive).
const DefaultSparkStartHour = 6

// DefaultSparkEndHour is the local seed-window end (exclusive).
// Learned pref/hours sleep still skips fires inside this window. Work is not DND.
const DefaultSparkEndHour = 21

// DefaultSparkPrompt is the built-in horizon pool (one variant per line).
// PickSparkPrompt chooses randomly at fire time.
// Every line is looking-after-the-user work (aims / memory / live tools / user-model), not an empty chat ping.
const DefaultSparkPrompt = "" +
	"First: memory_recall aim/. If SELF.md has no north-star and recall is empty, recall subject aim/bootstrap. Already asked and no answer → [silent]. Never asked → ONE months-scale question (what should we be aiming at for the next few months?). Do not invent. memory_store fact subject aim/bootstrap that you asked.\n" +
	"If aims exist: replan today. In one response: cron_list plus live tools (calendar, mail, fitness — mcp_enable if off). A real empty calendar is a hole, not [silent]: ask ONE what they want on it today (lunch/dinner or a training plan); try to get something scheduled (ask first before writing events). Never agree-and-stop. Day shifted or off track → one sentence. Unchanged full day → [silent]. Ask-first: no email, spend, or posts.\n" +
	"Pick one north-star from SELF.md. memory_recall aim/<area> and cron_list in one response. If that aim has no wake, cron_schedule one (live-data jobs must name the tools and say not to invent numbers). If a live tool would update progress, call it. memory_store a short progress fact. Then [silent] unless a hole needs the human.\n" +
	"Audit cron_list against SELF.md north-stars and memory aim/. Cancel wakes that no longer match. Schedule missing ones. Reply [silent] if the board is already right.\n" +
	"Look at SELF.md aims plus [current time] and today's live context. If something is due, stalled, or the morning plan broke, do the work with tools (or schedule the next wake). Reply [silent] when the work is only for you.\n" +
	"Gym / fitness aim: [current time] + memory_recall aim/ + garmin (mcp_enable if off) in ONE response. Morning or midday and no workout yet → short joke or nudge in your voice (quote SELF.md). Evening and still nothing → disappointed-uncle about the miss, one question if useful. Workout logged → [silent] or one nod. Never a joke with zero tools. [silent] if they asked you not to nag gym. cron_list if a gym follow-up is already on the board. Calendar has training later → if the prep cue (scoop, leave) has no cron, cron_schedule it or ask once.\n" +
	"User-model: memory_recall pref/food pref/activity pref/sports. Empty category → ONE question (favorite food, what they like to do, a team). 3+ foods → what they ate lately OR search a restaurant that fits. Has a team → search a recent game, ask if they watched. Store answers memory_store preference (same subject replaces). Not a questionnaire. [silent] if they were just asked. aim/ and cron still apply if something is due.\n" +
	"Hours bootstrap: memory_recall pref/hours. Missing → ask sleep window, work window, extra quiet times (work is not DND). Store preference subject pref/hours as sleep: HH:MM-HH:MM / work: HH:MM-HH:MM / quiet: …. Then continue with aims or [silent]. If hours exist, do not ask again. cron_list if needed.\n" +
	"Empty day: pull calendar (mcp_enable if off) + memory_recall aim/ pref/food pref/calendar. A real empty calendar is a problem to solve. Ask ONE what they want on today — lunch/dinner, or a training plan? Propose one thing. Ask first before writing events. Never agree-and-stop. [silent] only if they already said leave the day empty.\n" +
	"Prep cue: calendar has training or a timed plan today. cron_list. Missing ping before it (scoop, leave, refuel) → memory_store follow/ + cron_schedule with memory_id, or ask once (ping you at 2?). A calendar event is not a Telegram reminder. [silent] if that cue is already on the board. memory_recall aim/."

// ParseSparkPrompts splits a prompt pool on newlines (literal \n is expanded).
// Empty input → default pool. One line → single prompt (commas/colons fine).
func ParseSparkPrompts(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		s = DefaultSparkPrompt
	}
	s = strings.ReplaceAll(s, `\n`, "\n")
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"Wake and make progress on north-star aims. Recall aim/, check cron_list, call tools or schedule wakes. Reply [silent] unless the human needs a message."}
	}
	return out
}

// PickSparkPrompt returns one prompt from a newline-separated pool (or the only entry).
func PickSparkPrompt(pool string) string {
	prompts := ParseSparkPrompts(pool)
	if len(prompts) == 1 {
		return prompts[0]
	}
	return prompts[rand.IntN(len(prompts))]
}

// SparkSpec is qty range + daily window.
//
// Expr is always the template: 4-6@06-21
type SparkSpec struct {
	Min, Max           int
	StartHour, EndHour int // local; window is [StartHour:00, EndHour:00)
}

// ParseSparkQty accepts "5" or "4-6".
func ParseSparkQty(s string) (qtyMin, qtyMax int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, fmt.Errorf("cron: empty spark qty")
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		qtyMin, err = strconv.Atoi(strings.TrimSpace(s[:i]))
		if err != nil {
			return 0, 0, fmt.Errorf("cron: bad spark qty %q", s)
		}
		qtyMax, err = strconv.Atoi(strings.TrimSpace(s[i+1:]))
		if err != nil {
			return 0, 0, fmt.Errorf("cron: bad spark qty %q", s)
		}
	} else {
		qtyMin, err = strconv.Atoi(s)
		if err != nil {
			return 0, 0, fmt.Errorf("cron: bad spark qty %q", s)
		}
		qtyMax = qtyMin
	}
	if qtyMin < 1 || qtyMax < qtyMin || qtyMax > 24 {
		return 0, 0, fmt.Errorf("cron: spark qty must be 1–24 with min<=max, got %d-%d", qtyMin, qtyMax)
	}
	return qtyMin, qtyMax, nil
}

// ParseSparkExpr parses a spark template expr (qty@HH-HH). Strips legacy #state if present.
func ParseSparkExpr(expr string) (SparkSpec, error) {
	expr = strings.TrimSpace(expr)
	if i := strings.IndexByte(expr, '#'); i >= 0 {
		expr = expr[:i]
	}
	parts := strings.Split(expr, "@")
	if len(parts) != 2 {
		return SparkSpec{}, fmt.Errorf("cron: bad spark expr %q (want qty@HH-HH)", expr)
	}
	qtyMin, qtyMax, err := ParseSparkQty(parts[0])
	if err != nil {
		return SparkSpec{}, err
	}
	hours := strings.Split(parts[1], "-")
	if len(hours) != 2 {
		return SparkSpec{}, fmt.Errorf("cron: bad spark window %q", parts[1])
	}
	start, err := strconv.Atoi(strings.TrimSpace(hours[0]))
	if err != nil {
		return SparkSpec{}, fmt.Errorf("cron: bad spark start hour")
	}
	end, err := strconv.Atoi(strings.TrimSpace(hours[1]))
	if err != nil {
		return SparkSpec{}, fmt.Errorf("cron: bad spark end hour")
	}
	if start < 0 || start > 23 || end < 1 || end > 24 || end <= start {
		return SparkSpec{}, fmt.Errorf("cron: spark window must be start 0–23, end 1–24, end>start (got %d-%d)", start, end)
	}
	return SparkSpec{Min: qtyMin, Max: qtyMax, StartHour: start, EndHour: end}, nil
}

// FormatSparkExpr renders the template.
func FormatSparkExpr(s SparkSpec) string {
	return fmt.Sprintf("%d-%d@%02d-%02d", s.Min, s.Max, s.StartHour, s.EndHour)
}

// PlanSparkPlannerNext returns when the daily planner should next wake
// (start of window today if still before it, otherwise tomorrow's start).
func PlanSparkPlannerNext(spec SparkSpec, loc *time.Location, now time.Time) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)
	startToday := windowStart(now, spec.StartHour, loc)
	if now.Before(startToday) {
		return startToday.UTC()
	}
	return addOneCalendarDay(startToday).UTC()
}

// PlanSparkDayTimes rolls qty in [min,max] and picks that many times spread across
// the remaining window [lo, end) so the day stays balanced and the minimum is hit.
func PlanSparkDayTimes(spec SparkSpec, loc *time.Location, now time.Time) (n int, times []time.Time, err error) {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)
	start := windowStart(now, spec.StartHour, loc)
	end := windowStart(now, spec.EndHour, loc)
	if !end.After(start) {
		return 0, nil, fmt.Errorf("cron: empty spark window")
	}
	lo := start
	if now.After(lo) {
		lo = now.Add(time.Minute)
	}
	if !end.After(lo) {
		return 0, nil, nil // nothing left today
	}

	n = sparkRollQty(spec.Min, spec.Max)
	span := end.Sub(lo)
	// If the remainder of the day is tiny, still place n times in what's left.
	slot := span / time.Duration(n)
	if slot < time.Minute {
		// Fit as many as the window allows at ~1m spacing, but never below min
		// when the full window (start→end) could have held min.
		maxFit := int(span / time.Minute)
		if maxFit < 1 {
			maxFit = 1
		}
		if n > maxFit {
			n = maxFit
		}
		if n < spec.Min {
			// Prefer hitting min by packing into the remaining window.
			n = spec.Min
			if n > maxFit && maxFit >= 1 {
				n = maxFit
			}
		}
		slot = span / time.Duration(n)
		if slot < time.Second {
			slot = time.Second
		}
	}

	times = make([]time.Time, 0, n)
	for i := 0; i < n; i++ {
		slotLo := lo.Add(slot * time.Duration(i))
		slotHi := slotLo.Add(slot)
		if i == n-1 || slotHi.After(end) {
			slotHi = end
		}
		if !slotHi.After(slotLo) {
			times = append(times, slotLo.UTC())
			continue
		}
		times = append(times, randTimeBetween(slotLo, slotHi).UTC())
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	return n, times, nil
}

func sparkRollQty(qtyMin, qtyMax int) int {
	if qtyMax <= qtyMin {
		return qtyMin
	}
	return qtyMin + rand.IntN(qtyMax-qtyMin+1)
}

func windowStart(day time.Time, hour int, loc *time.Location) time.Time {
	day = day.In(loc)
	return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, loc)
}

func randTimeBetween(lo, hi time.Time) time.Time {
	if !hi.After(lo) {
		return lo
	}
	span := hi.Sub(lo)
	sec := int64(span / time.Second)
	if sec <= 1 {
		return lo
	}
	return lo.Add(time.Duration(rand.Int64N(sec)) * time.Second)
}

// ParseSparkSchedule builds a planner job (KindSpark) waking at the next window start.
func ParseSparkSchedule(when string, startHour, endHour int, loc *time.Location, now time.Time) (Parsed, error) {
	when = strings.TrimSpace(when)
	when = strings.TrimPrefix(when, "spark:")
	var spec SparkSpec
	if strings.Contains(when, "@") {
		var err error
		spec, err = ParseSparkExpr(when)
		if err != nil {
			return Parsed{}, err
		}
	} else {
		qtyMin, qtyMax, err := ParseSparkQty(when)
		if err != nil {
			return Parsed{}, err
		}
		if startHour < 0 || endHour <= startHour {
			startHour, endHour = 6, 21
		}
		spec = SparkSpec{Min: qtyMin, Max: qtyMax, StartHour: startHour, EndHour: endHour}
	}
	if loc == nil {
		loc = time.UTC
	}
	return Parsed{
		Kind:     KindSpark,
		Expr:     FormatSparkExpr(spec),
		NextRun:  PlanSparkPlannerNext(spec, loc, now),
		Timezone: loc.String(),
	}, nil
}

// SparkPingParsed builds a one-shot spark ping at t.
func SparkPingParsed(t time.Time, tz string) Parsed {
	return Parsed{
		Kind:     KindSparkPing,
		Expr:     t.UTC().Format(time.RFC3339Nano),
		NextRun:  t.UTC(),
		Timezone: tz,
	}
}
