package aims

import (
	"context"
	"fmt"
	"time"
)

// BlockStats is adherence over an aim's stored span, clipped to today.
// Pct is Up/Days. A span that has not started is Days 0 and Pct 0.
type BlockStats struct {
	Days    int     `json:"days"`
	Up      int     `json:"up"`
	Against int     `json:"against"`
	Mean    float64 `json:"mean"`
	Pct     float64 `json:"pct"`
}

// BlockStatsAt reads the aim_block row and the day scores inside it.
// ok is false when no row is stored.
func (s *Store) BlockStatsAt(ctx context.Context, area string, now time.Time) (BlockStats, bool, error) {
	b, ok, err := s.Block(ctx, area)
	if err != nil || !ok {
		return BlockStats{}, false, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	today := now.In(s.loc).Format(dayLayout)
	to := b.ToDay
	if to > today {
		to = today
	}
	if b.FromDay > to {
		return BlockStats{}, true, nil
	}
	days, err := s.DayScores(ctx, area, b.FromDay, to)
	if err != nil {
		return BlockStats{}, false, err
	}
	var st BlockStats
	st.Days = len(days)
	var sum int
	for _, d := range days {
		sum += d.Score
		switch {
		case d.Score > 0:
			st.Up++
		case d.Score < 0:
			st.Against++
		}
	}
	if st.Days > 0 {
		st.Mean = float64(sum) / float64(st.Days)
		st.Pct = float64(st.Up) / float64(st.Days)
	}
	return st, true, nil
}

func errBadDay(day string) error {
	return fmt.Errorf("aims: bad day %q", day)
}
