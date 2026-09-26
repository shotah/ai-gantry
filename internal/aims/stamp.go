package aims

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// Suffix is the [aims] tail. Empty when the aim has no ledger rows.
func Suffix(st Stats) string {
	if !st.HasEvents {
		return ""
	}
	parts := []string{
		fmt.Sprintf("30d %s", formatMean(st.Rating30)),
		fmt.Sprintf("7d %s", formatSigned(st.Sum7)),
		fmt.Sprintf("streak %d", st.Streak),
	}
	if note := noteAge(st); note != "" {
		parts = append(parts, note)
	}
	return strings.Join(parts, " · ")
}

// Progress is the planner-turn block for one aim: rating line, an
// optional measurement, then a date grid. An empty day is "·".
func Progress(area string, days []DayScore, st Stats, ser *Series) string {
	head := Suffix(st)
	if head == "" {
		head = "no ledger yet"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s", area, head)
	if ser != nil && ser.OK {
		fmt.Fprintf(&b, "\n  %s latest %g %s", ser.Metric, ser.Latest, strings.TrimSpace(ser.Unit))
		if ser.N >= 2 {
			fmt.Fprintf(&b, " · slope %+.2f/d", ser.Slope)
		}
	}
	for _, d := range days {
		b.WriteByte('\n')
		if len(d.Events) == 0 {
			fmt.Fprintf(&b, "  %s ·", d.Day)
			continue
		}
		fmt.Fprintf(&b, "  %s %s", d.Day, formatSigned(d.Score))
		for _, ev := range d.Events {
			fmt.Fprintf(&b, " #%d %s", ev.ID, ev.What)
		}
	}
	return b.String()
}

// ProgressText is the [progress] block for the planner turn: five local
// days per area, rating, and a measurement when one was logged.
func (s *Store) ProgressText(ctx context.Context, areas []string, now time.Time) string {
	if s == nil || len(areas) == 0 {
		return ""
	}
	if now.IsZero() {
		now = time.Now()
	}
	today := now.In(s.loc).Format(dayLayout)
	from, err := addDays(today, -4)
	if err != nil {
		return ""
	}
	month, err := addDays(today, -29)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, area := range areas {
		area = strings.TrimSpace(area)
		if area == "" || area == "bootstrap" {
			continue
		}
		days, err := s.DayScores(ctx, area, from, today)
		if err != nil {
			continue
		}
		st, err := s.StatsAt(ctx, area, now)
		if err != nil {
			continue
		}
		var ser *Series
		if metric := latestMetric(days); metric != "" {
			got, err := s.SeriesAt(ctx, area, metric, month, today)
			if err == nil && got.OK {
				ser = &got
			}
		}
		if b.Len() == 0 {
			b.WriteString("[progress]")
		}
		b.WriteByte('\n')
		b.WriteString(Progress(area, days, st, ser))
	}
	return b.String()
}

func latestMetric(days []DayScore) string {
	for i := len(days) - 1; i >= 0; i-- {
		for j := len(days[i].Events) - 1; j >= 0; j-- {
			if m := strings.TrimSpace(days[i].Events[j].Metric); m != "" {
				return m
			}
		}
	}
	return ""
}

func noteAge(st Stats) string {
	if st.LastNote == "" || st.LastNoteAt.IsZero() {
		return ""
	}
	return st.LastNote + " " + channel.Age(time.Since(st.LastNoteAt))
}

func formatSigned(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprintf("%d", n)
}

func formatMean(v float64) string {
	s := fmt.Sprintf("%+.1f", v)
	if s == "+0.0" || s == "-0.0" {
		return "0.0"
	}
	return s
}
