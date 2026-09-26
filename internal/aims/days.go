package aims

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DayScore is one local calendar day for an aim. Missing days are Score 0
// and have no events.
type DayScore struct {
	Day    string
	Score  int
	Events []Event
}

// DayScores returns one row per calendar day from through to, inclusive,
// in the store's timezone. Scores are the sum of live events that day,
// clamped to ScoreMin..ScoreMax.
func (s *Store) DayScores(ctx context.Context, area, fromDay, toDay string) ([]DayScore, error) {
	area = strings.TrimSpace(area)
	if area == "" {
		return nil, fmt.Errorf("aims: empty area")
	}
	if err := parseDay(fromDay); err != nil {
		return nil, err
	}
	if err := parseDay(toDay); err != nil {
		return nil, err
	}
	if fromDay > toDay {
		return nil, fmt.Errorf("aims: from %s is after to %s", fromDay, toDay)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.day, e.what, e.ref, e.metric, e.value, e.unit, e.source, e.created_at, e.superseded_by,
		       s.score, s.note
		FROM aim_event e
		JOIN aim_score s ON s.event_id = e.id
		WHERE e.superseded_by IS NULL AND s.area = ? AND e.day >= ? AND e.day <= ?
		ORDER BY e.day, e.id`, area, fromDay, toDay)
	if err != nil {
		return nil, fmt.Errorf("aims: days: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byDay := map[string]*DayScore{}
	for rows.Next() {
		var ev Event
		var val sql.NullFloat64
		var created string
		var sup sql.NullInt64
		var score int
		var note string
		if err := rows.Scan(&ev.ID, &ev.Day, &ev.What, &ev.Ref, &ev.Metric, &val, &ev.Unit, &ev.Source, &created, &sup, &score, &note); err != nil {
			return nil, err
		}
		if val.Valid {
			v := val.Float64
			ev.Value = &v
		}
		if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
			ev.CreatedAt = t
		}
		ev.Scores = []Score{{Area: area, Value: score, Note: note}}
		d := byDay[ev.Day]
		if d == nil {
			d = &DayScore{Day: ev.Day}
			byDay[ev.Day] = d
		}
		d.Score += score
		d.Events = append(d.Events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []DayScore
	for day := fromDay; day <= toDay; {
		d := DayScore{Day: day}
		if hit := byDay[day]; hit != nil {
			d.Score = clamp(hit.Score)
			d.Events = hit.Events
		}
		out = append(out, d)
		next, err := nextDay(day)
		if err != nil {
			return nil, err
		}
		day = next
	}
	return out, nil
}

func clamp(n int) int {
	if n < ScoreMin {
		return ScoreMin
	}
	if n > ScoreMax {
		return ScoreMax
	}
	return n
}

func nextDay(day string) (string, error) {
	t, err := time.Parse(dayLayout, day)
	if err != nil {
		return "", fmt.Errorf("aims: bad day %q", day)
	}
	return t.AddDate(0, 0, 1).Format(dayLayout), nil
}

func addDays(day string, n int) (string, error) {
	t, err := time.Parse(dayLayout, day)
	if err != nil {
		return "", fmt.Errorf("aims: bad day %q", day)
	}
	return t.AddDate(0, 0, n).Format(dayLayout), nil
}
