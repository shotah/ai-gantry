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

type samplePoint struct {
	day string
	v   float64
}

// slopePerDay is ordinary least squares of value against days since the first point.
func slopePerDay(pts []samplePoint) float64 {
	t0, err := time.Parse(dayLayout, pts[0].day)
	if err != nil {
		return 0
	}
	var n, sumX, sumY, sumXY, sumXX float64
	for _, p := range pts {
		t, err := time.Parse(dayLayout, p.day)
		if err != nil {
			continue
		}
		x := t.Sub(t0).Hours() / 24
		n++
		sumX += x
		sumY += p.v
		sumXY += x * p.v
		sumXX += x * x
	}
	den := n*sumXX - sumX*sumX
	if den == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / den
}
