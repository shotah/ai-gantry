package aims

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const weekCap = 13

// Measure is the mean of one metric in one unit over a week.
// Metric is the logged name; two units of the same metric are two
// measures.
type Measure struct {
	Metric string  `json:"metric"`
	Mean   float64 `json:"mean"`
	Unit   string  `json:"unit"`
	N      int     `json:"n"`
}

// Week is one Sunday-start local week of an aim, from the week of its
// first event. Mean includes zero days inside the aim's life. Days
// before that event, and days after today, are not in the mean.
type Week struct {
	Start   string
	Mean    float64
	Up      int
	Against int
	Metrics map[string]Measure
}

// Weeks returns Sunday-start weeks from the aim's first live event
// through the week that contains now, oldest first. More than 13
// weeks drops the oldest. No events is a nil slice.
func (s *Store) Weeks(ctx context.Context, area string, now time.Time) ([]Week, error) {
	area = strings.TrimSpace(area)
	if area == "" {
		return nil, fmt.Errorf("aims: empty area")
	}
	if now.IsZero() {
		now = time.Now()
	}
	today := now.In(s.loc).Format(dayLayout)
	first, ok, err := s.firstDay(ctx, area)
	if err != nil || !ok {
		return nil, err
	}
	start, err := weekStart(first, s.loc)
	if err != nil {
		return nil, err
	}
	end, err := weekStart(today, s.loc)
	if err != nil {
		return nil, err
	}
	var starts []string
	for d := start; d <= end; {
		starts = append(starts, d)
		next, err := addDays(d, 7)
		if err != nil {
			return nil, err
		}
		d = next
	}
	if len(starts) > weekCap {
		starts = starts[len(starts)-weekCap:]
	}
	out := make([]Week, 0, len(starts))
	for _, ws := range starts {
		we, err := addDays(ws, 6)
		if err != nil {
			return nil, err
		}
		from := ws
		if from < first {
			from = first
		}
		to := we
		if to > today {
			to = today
		}
		if from > to {
			continue
		}
		days, err := s.DayScores(ctx, area, from, to)
		if err != nil {
			return nil, err
		}
		w := Week{Start: ws, Metrics: metricsFrom(days)}
		var sum int
		for _, d := range days {
			sum += d.Score
			switch {
			case d.Score > 0:
				w.Up++
			case d.Score < 0:
				w.Against++
			}
		}
		if len(days) > 0 {
			w.Mean = float64(sum) / float64(len(days))
		}
		out = append(out, w)
	}
	return out, nil
}

// WeekSlope is ols of week means against week index. ok is false
// under two weeks.
func WeekSlope(weeks []Week) (float64, bool) {
	if len(weeks) < 2 {
		return 0, false
	}
	xs := make([]float64, len(weeks))
	ys := make([]float64, len(weeks))
	for i, w := range weeks {
		xs[i] = float64(i)
		ys[i] = w.Mean
	}
	return ols(xs, ys), true
}

func (s *Store) firstDay(ctx context.Context, area string) (string, bool, error) {
	var day sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT MIN(e.day)
		FROM aim_event e
		JOIN aim_score sc ON sc.event_id = e.id
		WHERE e.superseded_by IS NULL AND sc.area = ?`, area).Scan(&day)
	if err != nil {
		return "", false, err
	}
	if !day.Valid || day.String == "" {
		return "", false, nil
	}
	return day.String, true, nil
}

func weekStart(day string, loc *time.Location) (string, error) {
	if loc == nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation(dayLayout, day, loc)
	if err != nil {
		return "", errBadDay(day)
	}
	start := t.AddDate(0, 0, -int(t.Weekday()))
	return start.Format(dayLayout), nil
}

func isWeekStart(now time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = time.UTC
	}
	return now.In(loc).Weekday() == time.Sunday
}

func metricsFrom(days []DayScore) map[string]Measure {
	type acc struct {
		metric string
		unit   string
		sum    float64
		n      int
	}
	got := map[string]*acc{}
	for _, d := range days {
		for _, ev := range d.Events {
			metric := strings.TrimSpace(ev.Metric)
			if ev.Value == nil || metric == "" {
				continue
			}
			key := metric + "\x1f" + ev.Unit
			a := got[key]
			if a == nil {
				a = &acc{metric: metric, unit: ev.Unit}
				got[key] = a
			}
			a.sum += *ev.Value
			a.n++
		}
	}
	if len(got) == 0 {
		return nil
	}
	out := make(map[string]Measure, len(got))
	for k, a := range got {
		out[k] = Measure{Metric: a.metric, Mean: a.sum / float64(a.n), Unit: a.unit, N: a.n}
	}
	return out
}
