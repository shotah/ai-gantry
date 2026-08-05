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

// KindSparkPing is one presence ping created by the planner (one-shot).
const KindSparkPing = "spark_ping"

// DefaultSparkPrompt is used when SPARK_PROMPT is empty but spark is enabled.
// One variant per line; PickSparkPrompt chooses randomly at fire time.
const DefaultSparkPrompt = "" +
	"Share a fascinating, hyper-specific lab-energy curiosity — biometric, recovery, focus, or craft. No tools. Under 3 sentences. No dad jokes or canned games.\n" +
	"Offer a sharp, dry observation about tech or everyday friction. No tools. Under 3 sentences. Present, not preachy.\n" +
	"Drop a slightly sarcastic take on outdoor culture or training habits. No tools. Under 3 sentences. Authentic, not a template game.\n" +
	"Send a raw midday check-in with one concrete, non-generic insight. No tools. Under 3 sentences. Skip jokes and internet games.\n" +
	"Notice something oddly specific about modern work or tools, then land it lightly. No tools. Under 3 sentences.\n" +
	"Share one athletic-recovery or focus insight that feels researched, not motivational-poster. No tools. Under 3 sentences.\n" +
	"Make a dry, present observation about weather, place, or the weirdness of being online all day. No tools. Under 3 sentences.\n" +
	"Offer a curious craft or systems thought — specific enough to feel real. No tools. Under 3 sentences. No would-you-rathers or two-truths."

// ParseSparkPrompts splits a prompt pool on newlines (literal \n in env is expanded).
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
		return []string{"Be present with a short joke or check-in. No tools. 1–3 sentences."}
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

// TemplateExpr is an alias for FormatSparkExpr (API compat with EnsureSpark).
func (s SparkSpec) TemplateExpr() string { return FormatSparkExpr(s) }

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

// InSparkWindow reports whether t is inside [start, end) on that local day.
func InSparkWindow(spec SparkSpec, t time.Time, loc *time.Location) bool {
	t = t.In(loc)
	start := windowStart(t, spec.StartHour, loc)
	end := windowStart(t, spec.EndHour, loc)
	return !t.Before(start) && t.Before(end)
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
