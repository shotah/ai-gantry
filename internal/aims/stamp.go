package aims

import (
	"context"
	"fmt"
	"math"
	"sort"
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
		if line := s.blockLine(ctx, area, now); line != "" {
			b.WriteString("\n  " + line)
		}
		if isWeekStart(now, s.loc) {
			if extra := s.weekLines(ctx, area, now); extra != "" {
				b.WriteByte('\n')
				b.WriteString(extra)
			}
		}
	}
	if isWeekStart(now, s.loc) {
		for _, line := range s.crossLines(ctx, areas, now) {
			b.WriteString("\n" + line)
		}
	}
	return b.String()
}

// AreaTrend is the /aims <area> header under the rating: block, the
// last eight week means, the slope, and the effect line when it passes.
func (s *Store) AreaTrend(ctx context.Context, area string, now time.Time) string {
	if s == nil {
		return ""
	}
	var lines []string
	if line := s.blockLine(ctx, area, now); line != "" {
		lines = append(lines, line)
	}
	if extra := s.weekLines(ctx, area, now); extra != "" {
		lines = append(lines, extra)
	}
	return strings.Join(lines, "\n")
}

// BoardTrend is the /aims footer: one block line per area, then up to
// three cross-aim correlations.
func (s *Store) BoardTrend(ctx context.Context, areas []string, now time.Time) string {
	if s == nil {
		return ""
	}
	var lines []string
	for _, area := range areas {
		area = strings.TrimSpace(area)
		if area == "" || area == "bootstrap" {
			continue
		}
		if line := s.blockLine(ctx, area, now); line != "" {
			lines = append(lines, area+" "+line)
		}
	}
	lines = append(lines, s.crossLines(ctx, areas, now)...)
	return strings.Join(lines, "\n")
}

func (s *Store) blockLine(ctx context.Context, area string, now time.Time) string {
	st, ok, err := s.BlockStatsAt(ctx, area, now)
	if err != nil || !ok || st.Days == 0 {
		return ""
	}
	return fmt.Sprintf("block %d/%d (%.0f%%)", st.Up, st.Days, st.Pct*100)
}

func (s *Store) weekLines(ctx context.Context, area string, now time.Time) string {
	weeks, err := s.Weeks(ctx, area, now)
	if err != nil || len(weeks) == 0 {
		return ""
	}
	var lines []string
	if line := formatWeeks(weeks); line != "" {
		lines = append(lines, "  "+line)
	}
	if line := formatWeekSlope(weeks); line != "" {
		lines = append(lines, "  "+line)
	}
	c, ok := Effect(area, weeks)
	if line := formatEffect(c, ok); line != "" {
		lines = append(lines, "  "+line)
	}
	return strings.Join(lines, "\n")
}

func (s *Store) crossLines(ctx context.Context, areas []string, now time.Time) []string {
	hits := s.crossHits(ctx, areas, now)
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.line
	}
	return out
}

type crossHit struct {
	c    Corr
	abs  float64
	line string
}

func (s *Store) crossHits(ctx context.Context, areas []string, now time.Time) []crossHit {
	capped := capAreas(areas)
	var hits []crossHit
	for _, a := range capped {
		for _, b := range capped {
			if a == b {
				continue
			}
			c, ok, err := s.NextDay(ctx, a, b, now)
			if err != nil {
				continue
			}
			line := formatCross(c, ok)
			if line == "" {
				continue
			}
			hits = append(hits, crossHit{c: c, abs: math.Abs(c.R), line: line})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].abs != hits[j].abs {
			return hits[i].abs > hits[j].abs
		}
		return hits[i].line < hits[j].line
	})
	if len(hits) > 3 {
		hits = hits[:3]
	}
	return hits
}

func capAreas(areas []string) []string {
	var out []string
	for _, a := range areas {
		a = strings.TrimSpace(a)
		if a == "" || a == "bootstrap" {
			continue
		}
		out = append(out, a)
		if len(out) == 5 {
			break
		}
	}
	return out
}

func formatWeeks(weeks []Week) string {
	shown := weeks
	if len(shown) > 8 {
		shown = shown[len(shown)-8:]
	}
	parts := make([]string, len(shown))
	for i, w := range shown {
		parts[i] = formatMean(w.Mean)
	}
	return "weeks: " + strings.Join(parts, " ")
}

func formatWeekSlope(weeks []Week) string {
	slope, ok := WeekSlope(weeks)
	if !ok {
		return ""
	}
	return fmt.Sprintf("slope %+.2f/wk", slope)
}

func formatEffect(c Corr, ok bool) string {
	if !StampCorr(ok, c.R) {
		return ""
	}
	return fmt.Sprintf("effect r=%+.2f (%d)", c.R, c.N)
}

func formatCross(c Corr, ok bool) string {
	if !StampCorr(ok, c.R) {
		return ""
	}
	return fmt.Sprintf("%s → next-day %s r=%+.2f (%d)", c.A, c.B, c.R, c.N)
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
