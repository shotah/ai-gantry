package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule kinds stored on job rows.
const (
	KindOnce  = "once"
	KindDaily = "daily"
	KindEvery = "every"
)

// Parsed is a normalized schedule.
type Parsed struct {
	Kind     string
	Expr     string // once: RFC3339; daily: HH:MM; every: duration string
	NextRun  time.Time
	Timezone string
}

// ParseSchedule interprets tool arguments into a concrete next run.
//
// when examples:
//   - RFC3339 timestamp
//   - "15:04" / "3:04PM" (today or tomorrow in loc)
//   - "in 30m" / "in 2h"
//
// repeat: "once" (default), "daily", or "every:30m" / "every:1h"
func ParseSchedule(when, repeat string, loc *time.Location, now time.Time) (Parsed, error) {
	when = strings.TrimSpace(when)
	repeat = strings.TrimSpace(strings.ToLower(repeat))
	if when == "" {
		return Parsed{}, fmt.Errorf("cron: when is required")
	}
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)

	if repeat == "" {
		repeat = KindOnce
	}

	// every:DURATION as repeat shorthand (when may be empty of meaning — use now+dur as first)
	if strings.HasPrefix(repeat, "every:") || strings.HasPrefix(when, "every:") {
		spec := repeat
		if strings.HasPrefix(when, "every:") {
			spec = when
		}
		durStr := strings.TrimSpace(strings.TrimPrefix(spec, "every:"))
		d, err := time.ParseDuration(durStr)
		if err != nil || d < time.Minute {
			return Parsed{}, fmt.Errorf("cron: invalid every duration %q (min 1m)", durStr)
		}
		next := now.Add(d)
		return Parsed{Kind: KindEvery, Expr: d.String(), NextRun: next.UTC(), Timezone: loc.String()}, nil
	}

	if strings.HasPrefix(when, "in ") {
		d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(when, "in ")))
		if err != nil || d <= 0 {
			return Parsed{}, fmt.Errorf("cron: invalid relative when %q", when)
		}
		next := now.Add(d)
		if repeat == KindDaily {
			return dailyAt(next.In(loc), loc, now)
		}
		if repeat != KindOnce {
			return Parsed{}, fmt.Errorf("cron: repeat %q incompatible with relative when", repeat)
		}
		return Parsed{Kind: KindOnce, Expr: next.UTC().Format(time.RFC3339Nano), NextRun: next.UTC(), Timezone: loc.String()}, nil
	}

	if t, err := parseAbsolute(when, loc, now); err == nil {
		switch repeat {
		case KindOnce:
			if !t.After(now) {
				return Parsed{}, fmt.Errorf("cron: when %s is not in the future", t.Format(time.RFC3339))
			}
			return Parsed{Kind: KindOnce, Expr: t.UTC().Format(time.RFC3339Nano), NextRun: t.UTC(), Timezone: loc.String()}, nil
		case KindDaily:
			return dailyAt(t.In(loc), loc, now)
		default:
			return Parsed{}, fmt.Errorf("cron: invalid repeat %q", repeat)
		}
	}

	return Parsed{}, fmt.Errorf("cron: cannot parse when %q", when)
}

func parseAbsolute(when string, loc *time.Location, now time.Time) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, when); err == nil {
		return t.In(loc), nil
	}
	if t, err := time.Parse(time.RFC3339, when); err == nil {
		return t.In(loc), nil
	}
	layouts := []string{"15:04", "3:04PM", "3:04pm", "15:04:05"}
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, when, loc)
		if err != nil {
			continue
		}
		cand := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
		if !cand.After(now) {
			cand = cand.Add(24 * time.Hour)
		}
		return cand, nil
	}
	return time.Time{}, fmt.Errorf("unparsed")
}

func dailyAt(t time.Time, loc *time.Location, now time.Time) (Parsed, error) {
	t = t.In(loc)
	now = now.In(loc)
	expr := fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
	next := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return Parsed{Kind: KindDaily, Expr: expr, NextRun: next.UTC(), Timezone: loc.String()}, nil
}

// AdvanceNext returns the following run time after a successful fire.
// For once, ok=false (job should be disabled).
func AdvanceNext(kind, expr, tz string, from time.Time) (next time.Time, ok bool, err error) {
	loc, err := loadTZ(tz)
	if err != nil {
		return time.Time{}, false, err
	}
	from = from.In(loc)
	switch kind {
	case KindOnce:
		return time.Time{}, false, nil
	case KindDaily:
		parts := strings.Split(expr, ":")
		if len(parts) != 2 {
			return time.Time{}, false, fmt.Errorf("cron: bad daily expr %q", expr)
		}
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		next = time.Date(from.Year(), from.Month(), from.Day(), h, m, 0, 0, loc)
		if !next.After(from) {
			next = next.Add(24 * time.Hour)
		}
		return next.UTC(), true, nil
	case KindEvery:
		d, err := time.ParseDuration(expr)
		if err != nil {
			return time.Time{}, false, err
		}
		return from.Add(d).UTC(), true, nil
	default:
		return time.Time{}, false, fmt.Errorf("cron: unknown kind %q", kind)
	}
}

func loadTZ(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "UTC") {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("cron: timezone %q: %w", name, err)
	}
	return loc, nil
}
