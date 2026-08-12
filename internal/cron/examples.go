package cron

import (
	"fmt"
	"strings"
	"time"
)

// KindExamples is the daily planner for capability-example (training wheels) pings.
const KindExamples = "examples"

// KindExamplesPing is one propose-only example ping created by the planner.
const KindExamplesPing = "examples_ping"

// ParseExamplesSchedule builds a KindExamples planner (same qty@HH-HH shape as spark).
func ParseExamplesSchedule(when string, startHour, endHour int, loc *time.Location, now time.Time) (Parsed, error) {
	when = strings.TrimSpace(when)
	when = strings.TrimPrefix(when, "examples:")
	var spec SparkSpec
	if strings.Contains(when, "@") {
		var err error
		spec, err = ParseSparkExpr(when)
		if err != nil {
			return Parsed{}, fmt.Errorf("cron: examples: %w", err)
		}
	} else {
		qtyMin, qtyMax, err := ParseSparkQty(when)
		if err != nil {
			return Parsed{}, fmt.Errorf("cron: examples: %w", err)
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
		Kind:     KindExamples,
		Expr:     FormatSparkExpr(spec),
		NextRun:  PlanSparkPlannerNext(spec, loc, now),
		Timezone: loc.String(),
	}, nil
}

// ExamplesPingParsed builds a one-shot examples ping at t.
func ExamplesPingParsed(t time.Time, tz string) Parsed {
	return Parsed{
		Kind:     KindExamplesPing,
		Expr:     t.UTC().Format(time.RFC3339Nano),
		NextRun:  t.UTC(),
		Timezone: tz,
	}
}
