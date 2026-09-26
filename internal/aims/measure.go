package aims

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Series is one measurement (weight, hrv) under an aim, in the unit it
// was logged. Mixed units are left as logged; Slope is per calendar day.
type Series struct {
	Metric string
	Unit   string
	Latest float64
	Mean   float64
	Slope  float64
	N      int
	OK     bool
}

// SeriesAt summarizes live measurements for area+metric between the days.
func (s *Store) SeriesAt(ctx context.Context, area, metric, fromDay, toDay string) (Series, error) {
	area = strings.TrimSpace(area)
	metric = strings.TrimSpace(metric)
	if area == "" || metric == "" {
		return Series{}, fmt.Errorf("aims: area and metric are required")
	}
	if err := parseDay(fromDay); err != nil {
		return Series{}, err
	}
	if err := parseDay(toDay); err != nil {
		return Series{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.day, e.value, e.unit
		FROM aim_event e
		JOIN aim_score s ON s.event_id = e.id
		WHERE e.superseded_by IS NULL AND s.area = ? AND e.metric = ?
		  AND e.value IS NOT NULL AND e.day >= ? AND e.day <= ?
		ORDER BY e.day, e.id`, area, metric, fromDay, toDay)
	if err != nil {
		return Series{}, fmt.Errorf("aims: series: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ser Series
	ser.Metric = metric
	var pts []samplePoint
	for rows.Next() {
		var day string
		var val sql.NullFloat64
		var unit string
		if err := rows.Scan(&day, &val, &unit); err != nil {
			return Series{}, err
		}
		if !val.Valid {
			continue
		}
		if ser.Unit == "" {
			ser.Unit = unit
		}
		pts = append(pts, samplePoint{day: day, v: val.Float64})
	}
	if err := rows.Err(); err != nil {
		return Series{}, err
	}
	if len(pts) == 0 {
		return ser, nil
	}
	ser.OK = true
	ser.N = len(pts)
	ser.Latest = pts[len(pts)-1].v
	var sum float64
	for _, p := range pts {
		sum += p.v
	}
	ser.Mean = sum / float64(len(pts))
	if len(pts) >= 2 {
		ser.Slope = slopePerDay(pts)
	}
	return ser, nil
}

// ols is ordinary least squares of y on x. A zero denominator (one
// point, or every x equal) is slope 0.
func ols(xs, ys []float64) float64 {
	n := float64(len(xs))
	if n == 0 || len(ys) != len(xs) {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumXX += xs[i] * xs[i]
	}
	den := n*sumXX - sumX*sumX
	if den == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / den
}

type samplePoint struct {
	day string
	v   float64
}

// slopePerDay is ols of value against days since the first point.
func slopePerDay(pts []samplePoint) float64 {
	if len(pts) == 0 {
		return 0
	}
	t0, err := time.Parse(dayLayout, pts[0].day)
	if err != nil {
		return 0
	}
	xs := make([]float64, 0, len(pts))
	ys := make([]float64, 0, len(pts))
	for _, p := range pts {
		t, err := time.Parse(dayLayout, p.day)
		if err != nil {
			continue
		}
		xs = append(xs, t.Sub(t0).Hours()/24)
		ys = append(ys, p.v)
	}
	return ols(xs, ys)
}
