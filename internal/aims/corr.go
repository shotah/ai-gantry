package aims

import (
	"context"
	"math"
	"time"
)

const (
	corrMinN = 8
	corrMinR = 0.3
)

// Corr is one Pearson result. B set means aim A against aim B the next
// day. B empty means the aim's score against its own measurement.
type Corr struct {
	A      string  `json:"a"`
	B      string  `json:"b"`
	Metric string  `json:"metric"`
	R      float64 `json:"r"`
	N      int     `json:"n"`
}

// Pearson is the correlation of two equal-length series. ok is false
// under 8 points, unequal lengths, or a side with zero variance.
func Pearson(xs, ys []float64) (float64, bool) {
	n := len(xs)
	if n < corrMinN || len(ys) != n {
		return 0, false
	}
	var sumX, sumY float64
	for i := 0; i < n; i++ {
		sumX += xs[i]
		sumY += ys[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)
	var sxx, syy, sxy float64
	for i := 0; i < n; i++ {
		dx := xs[i] - meanX
		dy := ys[i] - meanY
		sxx += dx * dx
		syy += dy * dy
		sxy += dx * dy
	}
	if sxx == 0 || syy == 0 {
		return 0, false
	}
	return sxy / math.Sqrt(sxx*syy), true
}

// StampCorr reports whether a correlation is strong enough to show.
// A missing line is the "too early" case; the words are not stamped.
func StampCorr(ok bool, r float64) bool {
	return ok && math.Abs(r) >= corrMinR
}

// Effect correlates weekly mean score with the weekly mean of the
// aim's latest metric, one unit. ok is Pearson's gate, not the stamp
// floor.
func Effect(area string, weeks []Week) (Corr, bool) {
	metric, unit := latestMeasure(weeks)
	if metric == "" {
		return Corr{A: area}, false
	}
	var xs, ys []float64
	for _, w := range weeks {
		m, ok := measureMatch(w, metric, unit)
		if !ok {
			continue
		}
		xs = append(xs, w.Mean)
		ys = append(ys, m.Mean)
	}
	r, ok := Pearson(xs, ys)
	return Corr{A: area, Metric: metric, R: r, N: len(xs)}, ok
}

// NextDay correlates aim a's score on days a was logged with aim b's
// score the next day, over the 90 local days ending today. A missing
// b day is 0. Days a was not logged do not count. ok is Pearson's gate.
func (s *Store) NextDay(ctx context.Context, a, b string, now time.Time) (Corr, bool, error) {
	if now.IsZero() {
		now = time.Now()
	}
	today := now.In(s.loc).Format(dayLayout)
	from, err := addDays(today, -89)
	if err != nil {
		return Corr{}, false, err
	}
	daysA, err := s.DayScores(ctx, a, from, today)
	if err != nil {
		return Corr{}, false, err
	}
	daysB, err := s.DayScores(ctx, b, from, today)
	if err != nil {
		return Corr{}, false, err
	}
	byB := make(map[string]int, len(daysB))
	for _, d := range daysB {
		byB[d.Day] = d.Score
	}
	var xs, ys []float64
	for _, d := range daysA {
		if len(d.Events) == 0 {
			continue
		}
		next, err := nextDay(d.Day)
		if err != nil {
			return Corr{}, false, err
		}
		if next > today {
			continue
		}
		xs = append(xs, float64(d.Score))
		ys = append(ys, float64(byB[next]))
	}
	r, ok := Pearson(xs, ys)
	return Corr{A: a, B: b, R: r, N: len(xs)}, ok, nil
}

func latestMeasure(weeks []Week) (metric, unit string) {
	for i := len(weeks) - 1; i >= 0; i-- {
		var best *Measure
		for _, m := range weeks[i].Metrics {
			if m.N == 0 || m.Metric == "" {
				continue
			}
			if betterMeasure(m, best) {
				mm := m
				best = &mm
			}
		}
		if best != nil {
			return best.Metric, best.Unit
		}
	}
	return "", ""
}

func betterMeasure(m Measure, best *Measure) bool {
	if best == nil || m.N > best.N {
		return true
	}
	if m.N < best.N {
		return false
	}
	if m.Metric != best.Metric {
		return m.Metric < best.Metric
	}
	return m.Unit < best.Unit
}

func measureMatch(w Week, metric, unit string) (Measure, bool) {
	for _, m := range w.Metrics {
		if m.Metric == metric && m.Unit == unit {
			return m, true
		}
	}
	return Measure{}, false
}
