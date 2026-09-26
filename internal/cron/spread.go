package cron

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Spread is how many one-shot jobs to place in a local hour window
// (qty@HH-HH). Callers pick the job kind. The daily planner is not this:
// it runs once, at one clock time.
type Spread struct {
	Min, Max           int
	StartHour, EndHour int // local; window is [StartHour:00, EndHour:00)
}

// ParseQty accepts "5" or "1-2".
func ParseQty(s string) (qtyMin, qtyMax int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, fmt.Errorf("cron: empty qty")
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		qtyMin, err = strconv.Atoi(strings.TrimSpace(s[:i]))
		if err != nil {
			return 0, 0, fmt.Errorf("cron: bad qty %q", s)
		}
		qtyMax, err = strconv.Atoi(strings.TrimSpace(s[i+1:]))
		if err != nil {
			return 0, 0, fmt.Errorf("cron: bad qty %q", s)
		}
	} else {
		qtyMin, err = strconv.Atoi(s)
		if err != nil {
			return 0, 0, fmt.Errorf("cron: bad qty %q", s)
		}
		qtyMax = qtyMin
	}
	if qtyMin < 1 || qtyMax < qtyMin || qtyMax > 24 {
		return 0, 0, fmt.Errorf("cron: qty must be 1–24 with min<=max, got %d-%d", qtyMin, qtyMax)
	}
	return qtyMin, qtyMax, nil
}

// ParseSpread parses a qty@HH-HH template.
func ParseSpread(expr string) (Spread, error) {
	expr = strings.TrimSpace(expr)
	parts := strings.Split(expr, "@")
	if len(parts) != 2 {
		return Spread{}, fmt.Errorf("cron: bad spread %q (want qty@HH-HH)", expr)
	}
	qtyMin, qtyMax, err := ParseQty(parts[0])
	if err != nil {
		return Spread{}, err
	}
	hours := strings.Split(parts[1], "-")
	if len(hours) != 2 {
		return Spread{}, fmt.Errorf("cron: bad spread %q", parts[1])
	}
	start, err := strconv.Atoi(strings.TrimSpace(hours[0]))
	if err != nil {
		return Spread{}, fmt.Errorf("cron: bad start hour")
	}
	end, err := strconv.Atoi(strings.TrimSpace(hours[1]))
	if err != nil {
		return Spread{}, fmt.Errorf("cron: bad end hour")
	}
	if start < 0 || start > 23 || end < 1 || end > 24 || end <= start {
		return Spread{}, fmt.Errorf("cron: window must be start 0–23, end 1–24, end>start (got %d-%d)", start, end)
	}
	return Spread{Min: qtyMin, Max: qtyMax, StartHour: start, EndHour: end}, nil
}

// FormatSpread renders the template.
func FormatSpread(q Spread) string {
	return fmt.Sprintf("%d-%d@%02d-%02d", q.Min, q.Max, q.StartHour, q.EndHour)
}

// PlanSpreadNext is the next window start (today if still before it, else tomorrow).
func PlanSpreadNext(q Spread, loc *time.Location, now time.Time) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)
	startToday := windowStart(now, q.StartHour, loc)
	if now.Before(startToday) {
		return startToday.UTC()
	}
	return addOneCalendarDay(startToday).UTC()
}

// PlanSpreadTimes rolls qty in [min,max] and picks that many times
// across the remaining window.
func PlanSpreadTimes(q Spread, loc *time.Location, now time.Time) (n int, times []time.Time, err error) {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)
	start := windowStart(now, q.StartHour, loc)
	end := windowStart(now, q.EndHour, loc)
	if !end.After(start) {
		return 0, nil, fmt.Errorf("cron: empty window")
	}
	lo := start
	if now.After(lo) {
		lo = now.Add(time.Minute)
	}
	if !end.After(lo) {
		return 0, nil, nil
	}

	n = rollQty(q.Min, q.Max)
	span := end.Sub(lo)
	slot := span / time.Duration(n)
	if slot < time.Minute {
		maxFit := int(span / time.Minute)
		if maxFit < 1 {
			maxFit = 1
		}
		if n > maxFit {
			n = maxFit
		}
		if n < q.Min {
			n = q.Min
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

func rollQty(qtyMin, qtyMax int) int {
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

// ParseSpreadSchedule builds a planner job of kind. when is "5", "1-2", or
// "1-2@06-21". A bare qty uses startHour and endHour (6–21 when those are empty).
func ParseSpreadSchedule(kind, when string, startHour, endHour int, loc *time.Location, now time.Time) (Parsed, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return Parsed{}, fmt.Errorf("cron: spread kind is required")
	}
	when = strings.TrimSpace(when)
	var spec Spread
	if strings.Contains(when, "@") {
		var err error
		spec, err = ParseSpread(when)
		if err != nil {
			return Parsed{}, fmt.Errorf("cron: spread: %w", err)
		}
	} else {
		qtyMin, qtyMax, err := ParseQty(when)
		if err != nil {
			return Parsed{}, fmt.Errorf("cron: spread: %w", err)
		}
		if startHour < 0 || endHour <= startHour {
			startHour, endHour = 6, 21
		}
		spec = Spread{Min: qtyMin, Max: qtyMax, StartHour: startHour, EndHour: endHour}
	}
	if loc == nil {
		loc = time.UTC
	}
	return Parsed{
		Kind:     kind,
		Expr:     FormatSpread(spec),
		NextRun:  PlanSpreadNext(spec, loc, now),
		Timezone: loc.String(),
	}, nil
}

// OnceParsed builds a one-shot job of kind at t.
func OnceParsed(kind string, t time.Time, tz string) Parsed {
	return Parsed{
		Kind:     kind,
		Expr:     t.UTC().Format(time.RFC3339Nano),
		NextRun:  t.UTC(),
		Timezone: tz,
	}
}
