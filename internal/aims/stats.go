package aims

import (
	"context"
	"time"
)

// Stats is the kernel's read of one aim. Rating30 and Mean7 include
// missing days as 0. Streak is consecutive days with score > 0. An empty
// today (nothing logged yet) stays out of the run, so a planner morning
// can still see the days behind it. A day that was scored ≤ 0 ends it.
type Stats struct {
	Sum7, Sum14     int
	Up7, Against7   int
	Streak          int
	Rating30, Mean7 float64
	LastNote        string
	LastNoteAt      time.Time
	HasEvents       bool
}

// StatsAt computes the windows ending on now's local calendar day.
func (s *Store) StatsAt(ctx context.Context, area string, now time.Time) (Stats, error) {
	if now.IsZero() {
		now = time.Now()
	}
	today := now.In(s.loc).Format(dayLayout)
	from, err := addDays(today, -29)
	if err != nil {
		return Stats{}, err
	}
	days, err := s.DayScores(ctx, area, from, today)
	if err != nil {
		return Stats{}, err
	}
	return statsFrom(days), nil
}

func statsFrom(days []DayScore) Stats {
	var st Stats
	if len(days) == 0 {
		return st
	}
	var sum30 int
	for _, d := range days {
		sum30 += d.Score
		if len(d.Events) > 0 {
			st.HasEvents = true
		}
		for _, ev := range d.Events {
			if len(ev.Scores) == 0 || ev.Scores[0].Note == "" {
				continue
			}
			at := ev.CreatedAt
			if at.IsZero() {
				if t, err := time.Parse(dayLayout, ev.Day); err == nil {
					at = t
				}
			}
			if st.LastNote == "" || at.After(st.LastNoteAt) {
				st.LastNote = ev.Scores[0].Note
				st.LastNoteAt = at
			}
		}
	}
	st.Rating30 = float64(sum30) / float64(len(days))
	tail := func(n int) []DayScore {
		if len(days) <= n {
			return days
		}
		return days[len(days)-n:]
	}
	week := tail(7)
	var sum7 int
	for _, d := range week {
		sum7 += d.Score
		switch {
		case d.Score > 0:
			st.Up7++
		case d.Score < 0:
			st.Against7++
		}
	}
	st.Sum7 = sum7
	st.Mean7 = float64(sum7) / float64(len(week))
	fortnight := tail(14)
	for _, d := range fortnight {
		st.Sum14 += d.Score
	}
	end := len(days) - 1
	if end >= 0 && len(days[end].Events) == 0 {
		end--
	}
	for i := end; i >= 0; i-- {
		if days[i].Score <= 0 {
			break
		}
		st.Streak++
	}
	return st
}
