package aims_test

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/aims"
)

func TestDayScores_ClampAndMissing(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "training")
	for _, what := range []string{"am", "pm"} {
		if _, err := f.store.Log(ctx, aims.Event{What: what, Day: "2026-09-22"}, map[string]int{"training": 2}); err != nil {
			t.Fatal(err)
		}
	}
	days, err := f.store.DayScores(ctx, "training", "2026-09-21", "2026-09-23")
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 3 || days[0].Score != 0 || len(days[0].Events) != 0 {
		t.Fatalf("missing day: %+v", days[0])
	}
	if days[1].Score != aims.ScoreMax || len(days[1].Events) != 2 {
		t.Fatalf("clamp: %+v", days[1])
	}
	if days[2].Day != "2026-09-23" || days[2].Score != 0 {
		t.Fatalf("tail: %+v", days[2])
	}
}

func TestStats_ZeroDayEndsStreak(t *testing.T) {
	ctx := context.Background()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	f := openFixture(t, loc)
	seedAims(t, f.mem, "training")
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, loc)
	if _, err := f.store.Log(ctx, aims.Event{What: "session", Day: "2026-09-26"}, map[string]int{"training": 2}); err != nil {
		t.Fatal(err)
	}
	st, err := f.store.StatsAt(ctx, "training", now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Streak != 1 || st.Sum7 != 2 {
		t.Fatalf("an empty today keeps yesterday's run: %+v", st)
	}
	if _, err := f.store.Log(ctx, aims.Event{What: "planned rest", Day: "2026-09-27"}, map[string]int{"training": 0}); err != nil {
		t.Fatal(err)
	}
	st, err = f.store.StatsAt(ctx, "training", now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Streak != 0 {
		t.Fatalf("a scored 0 today ends the streak: %+v", st)
	}
	if _, err := f.store.Log(ctx, aims.Event{What: "session", Day: "2026-09-27"}, map[string]int{"training": 2}); err != nil {
		t.Fatal(err)
	}
	st, err = f.store.StatsAt(ctx, "training", now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Streak != 2 {
		t.Fatalf("streak=%d", st.Streak)
	}
}

func TestStats_RatingSparseMonth(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "training")
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	if _, err := f.store.Log(ctx, aims.Event{What: "pr", Day: "2026-09-27"}, map[string]int{"training": 3}); err != nil {
		t.Fatal(err)
	}
	st, err := f.store.StatsAt(ctx, "training", now)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(st.Rating30-0.1) > 0.001 {
		t.Fatalf("rating=%v", st.Rating30)
	}
	if got := aims.Suffix(st); got != "30d +0.1 · 7d +3 · streak 1" {
		t.Fatalf("suffix %q", got)
	}
}

func TestDayScores_MonthBoundary(t *testing.T) {
	ctx := context.Background()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	f := openFixture(t, loc)
	seedAims(t, f.mem, "training")
	if _, err := f.store.Log(ctx, aims.Event{What: "sept", Day: "2026-09-30"}, map[string]int{"training": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Log(ctx, aims.Event{What: "oct", Day: "2026-10-01"}, map[string]int{"training": 2}); err != nil {
		t.Fatal(err)
	}
	days, err := f.store.DayScores(ctx, "training", "2026-09-30", "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[0].Score != 1 || days[1].Score != 2 || days[1].Day != "2026-10-01" {
		t.Fatalf("days=%+v", days)
	}
}

func TestStats_FixtureWeek(t *testing.T) {
	ctx := context.Background()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	f := openFixture(t, loc)
	seedAims(t, f.mem, "drinking", "weight", "climbing")
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, loc)
	w := func(v float64) *float64 { return &v }
	rows := []struct {
		day    string
		what   string
		scores map[string]int
		note   string
		metric string
		value  *float64
	}{
		{"2026-09-21", "weighed in", map[string]int{"weight": 0}, "", "weight", w(193)},
		{"2026-09-21", "clean day", map[string]int{"drinking": 1}, "", "", nil},
		{"2026-09-22", "ran five miles", map[string]int{"weight": 1, "climbing": 1}, "", "", nil},
		{"2026-09-22", "team dinner: 3 beers and a burger", map[string]int{"drinking": -2, "weight": -1, "climbing": -1}, "", "", nil},
		{"2026-09-23", "skipped the morning session", map[string]int{"climbing": -1}, "", "", nil},
		{"2026-09-23", "weighed in", map[string]int{"weight": 0}, "", "weight", w(193.8)},
		{"2026-09-24", "8 boulders, sent the V5 project", map[string]int{"drinking": 1, "climbing": 3}, "", "", nil},
		{"2026-09-25", "asked what is in the way of mornings", map[string]int{"climbing": 0}, "asked", "", nil},
		{"2026-09-26", "long hike, no beer", map[string]int{"drinking": 1, "weight": 1, "climbing": 1}, "", "", nil},
		{"2026-09-27", "clean day, weighed in", map[string]int{"drinking": 1, "weight": 2}, "", "weight", w(191.9)},
	}
	for _, row := range rows {
		ev := aims.Event{What: row.what, Day: row.day, Note: row.note, Metric: row.metric, Value: row.value, Unit: "lb"}
		if _, err := f.store.Log(ctx, ev, row.scores); err != nil {
			t.Fatal(row.what, err)
		}
	}

	drink, err := f.store.StatsAt(ctx, "drinking", now)
	if err != nil {
		t.Fatal(err)
	}
	if drink.Sum7 != 2 || drink.Up7 != 4 || drink.Against7 != 1 || drink.Streak != 2 {
		t.Fatalf("drinking %+v", drink)
	}
	weight, err := f.store.StatsAt(ctx, "weight", now)
	if err != nil {
		t.Fatal(err)
	}
	if weight.Sum7 != 3 || weight.Streak != 2 {
		t.Fatalf("weight %+v", weight)
	}
	climb, err := f.store.StatsAt(ctx, "climbing", now)
	if err != nil {
		t.Fatal(err)
	}
	if climb.Sum7 != 3 || climb.Against7 != 1 || climb.Streak != 1 || climb.LastNote != "asked" {
		t.Fatalf("climbing %+v", climb)
	}

	ser, err := f.store.SeriesAt(ctx, "weight", "weight", "2026-09-21", "2026-09-27")
	if err != nil {
		t.Fatal(err)
	}
	if !ser.OK || ser.N != 3 || ser.Latest != 191.9 || ser.Unit != "lb" || ser.Slope >= 0 {
		t.Fatalf("series %+v", ser)
	}

	days, err := f.store.DayScores(ctx, "climbing", "2026-09-23", "2026-09-27")
	if err != nil {
		t.Fatal(err)
	}
	grid := aims.Progress("climbing", days, climb, nil)
	for _, part := range []string{"2026-09-25 0", "asked", "2026-09-27 ·"} {
		if !strings.Contains(grid, part) {
			t.Fatalf("grid missing %q:\n%s", part, grid)
		}
	}
	weeks, err := f.store.Weeks(ctx, "weight", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := aims.Effect("weight", weeks); ok {
		t.Fatal("one fixture week is too early for an effect")
	}
	if _, ok, err := f.store.NextDay(ctx, "drinking", "climbing", now); err != nil || ok {
		t.Fatalf("fixture next-day ok=%v err=%v", ok, err)
	}
}

func TestWeekSlope_FlatAndStep(t *testing.T) {
	flat := []aims.Week{{Mean: 1}, {Mean: 1}, {Mean: 1}}
	slope, ok := aims.WeekSlope(flat)
	if !ok || slope != 0 {
		t.Fatalf("flat slope=%v ok=%v", slope, ok)
	}
	if _, ok := aims.WeekSlope(flat[:1]); ok {
		t.Fatal("one week has no slope")
	}
	var step []aims.Week
	for i := 0; i < 8; i++ {
		m := 0.0
		if i >= 4 {
			m = 2
		}
		step = append(step, aims.Week{Mean: m})
	}
	slope, ok = aims.WeekSlope(step)
	if !ok || slope <= 0 {
		t.Fatalf("step slope=%v", slope)
	}
}

func TestWeeks_FirstEventClipsTheBucket(t *testing.T) {
	ctx := context.Background()
	loc := mustLA(t)
	f := openFixture(t, loc)
	seedAims(t, f.mem, "training")
	if _, err := f.store.Log(ctx, aims.Event{What: "session", Day: "2026-09-23"}, map[string]int{"training": 2}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, loc)
	weeks, err := f.store.Weeks(ctx, "training", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 1 || weeks[0].Start != "2026-09-20" || weeks[0].Up != 1 {
		t.Fatalf("weeks=%+v", weeks)
	}
	if math.Abs(weeks[0].Mean-0.5) > 0.001 {
		t.Fatalf("first bucket should be 4 days (mean 0.5), got %v", weeks[0].Mean)
	}
}

func TestWeeks_EmptyWeekStays(t *testing.T) {
	ctx := context.Background()
	loc := mustLA(t)
	f := openFixture(t, loc)
	seedAims(t, f.mem, "training")
	if _, err := f.store.Log(ctx, aims.Event{What: "start", Day: "2026-09-06"}, map[string]int{"training": 1}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, loc)
	weeks, err := f.store.Weeks(ctx, "training", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 3 || weeks[1].Start != "2026-09-13" || weeks[1].Mean != 0 {
		t.Fatalf("empty week dropped: %+v", weeks)
	}
}

func TestWeeks_MonthBoundaryIsOneBucket(t *testing.T) {
	ctx := context.Background()
	loc := mustLA(t)
	f := openFixture(t, loc)
	seedAims(t, f.mem, "training")
	for _, day := range []string{"2026-09-30", "2026-10-01"} {
		if _, err := f.store.Log(ctx, aims.Event{What: day, Day: day}, map[string]int{"training": 1}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	weeks, err := f.store.Weeks(ctx, "training", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 1 || weeks[0].Start != "2026-09-27" {
		t.Fatalf("weeks=%+v", weeks)
	}
}

func TestWeeks_MixedUnitsStaySplit(t *testing.T) {
	ctx := context.Background()
	loc := mustLA(t)
	f := openFixture(t, loc)
	seedAims(t, f.mem, "weight")
	lb, kg := 190.0, 86.0
	if _, err := f.store.Log(ctx, aims.Event{What: "lb", Day: "2026-09-21", Metric: "weight", Value: &lb, Unit: "lb"}, map[string]int{"weight": 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Log(ctx, aims.Event{What: "kg", Day: "2026-09-22", Metric: "weight", Value: &kg, Unit: "kg"}, map[string]int{"weight": 0}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, loc)
	weeks, err := f.store.Weeks(ctx, "weight", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 1 || len(weeks[0].Metrics) != 2 {
		t.Fatalf("metrics=%+v", weeks)
	}
	units := map[string]bool{}
	for _, m := range weeks[0].Metrics {
		units[m.Unit] = true
		if m.Metric != "weight" || m.N != 1 {
			t.Fatalf("measure=%+v", m)
		}
	}
	if !units["lb"] || !units["kg"] {
		t.Fatalf("units=%v", units)
	}
}

func TestBlockStats(t *testing.T) {
	ctx := context.Background()
	loc := mustLA(t)
	f := openFixture(t, loc)
	seedAims(t, f.mem, "training")
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, loc)
	if _, ok, err := f.store.BlockStatsAt(ctx, "training", now); err != nil || ok {
		t.Fatalf("no row ok=%v err=%v", ok, err)
	}
	if err := f.store.SetBlock(ctx, "training", "2026-09-23", "2026-10-02"); err != nil {
		t.Fatal(err)
	}
	for i, day := range []string{"2026-09-23", "2026-09-24", "2026-09-25", "2026-09-26"} {
		if _, err := f.store.Log(ctx, aims.Event{What: day, Day: day}, map[string]int{"training": 1 + i%2}); err != nil {
			t.Fatal(err)
		}
	}
	st, ok, err := f.store.BlockStatsAt(ctx, "training", now)
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if st.Days != 10 || st.Up != 4 || math.Abs(st.Pct-0.4) > 0.001 {
		t.Fatalf("stats=%+v", st)
	}
	future := time.Date(2026, 9, 30, 12, 0, 0, 0, loc)
	if err := f.store.SetBlock(ctx, "training", "2026-09-28", "2026-11-01"); err != nil {
		t.Fatal(err)
	}
	st, ok, err = f.store.BlockStatsAt(ctx, "training", future)
	if err != nil || !ok || st.Days != 3 {
		t.Fatalf("clipped %+v ok=%v err=%v", st, ok, err)
	}
	edge := time.Date(2026, 10, 2, 12, 0, 0, 0, loc)
	if err := f.store.SetBlock(ctx, "training", "2026-09-30", "2026-10-02"); err != nil {
		t.Fatal(err)
	}
	st, _, err = f.store.BlockStatsAt(ctx, "training", edge)
	if err != nil || st.Days != 3 {
		t.Fatalf("boundary %+v err=%v", st, err)
	}
}

func TestPearsonAndEffect(t *testing.T) {
	same := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	r, ok := aims.Pearson(same, same)
	if !ok || math.Abs(r-1) > 1e-9 {
		t.Fatalf("identical r=%v ok=%v", r, ok)
	}
	if _, ok := aims.Pearson(same[:7], same[:7]); ok {
		t.Fatal("7 points is too early")
	}
	xs := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	ys := []float64{0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1}
	r, ok = aims.Pearson(xs, ys)
	if !ok || math.Abs(r) >= 0.3 || aims.StampCorr(ok, r) {
		t.Fatalf("weak n=12 r=%v ok=%v should not stamp", r, ok)
	}
	if aims.StampCorr(true, 0.2) || !aims.StampCorr(true, 0.3) || aims.StampCorr(false, 1) {
		t.Fatal("stamp floor")
	}
	var falling []aims.Week
	for i := 0; i < 8; i++ {
		falling = append(falling, aims.Week{
			Mean: float64(i),
			Metrics: map[string]aims.Measure{
				"w": {Metric: "weight", Unit: "lb", Mean: float64(80 - i*5), N: 1},
			},
		})
	}
	c, ok := aims.Effect("weight", falling)
	if !ok || c.R >= 0 || c.N != 8 || c.Metric != "weight" {
		t.Fatalf("effect %+v ok=%v", c, ok)
	}
	if _, ok := aims.Effect("weight", falling[:7]); ok {
		t.Fatal("7-week effect should be too early")
	}
}

func TestNextDay_ScoredDaysOnly(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "drinking", "climbing")
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for _, day := range []string{"2026-09-01", "2026-09-10", "2026-09-20"} {
		if _, err := f.store.Log(ctx, aims.Event{What: "night", Day: day}, map[string]int{"drinking": -1}); err != nil {
			t.Fatal(err)
		}
	}
	c, ok, err := f.store.NextDay(ctx, "drinking", "climbing", now)
	if err != nil {
		t.Fatal(err)
	}
	if ok || c.N != 3 {
		t.Fatalf("n=%d ok=%v (zeros must not count)", c.N, ok)
	}
}

func TestProgress_WeekStartOnly(t *testing.T) {
	ctx := context.Background()
	loc := mustLA(t)
	f := openFixture(t, loc)
	seedAims(t, f.mem, "training", "weight")
	sundays := []struct {
		day   string
		score int
		lb    float64
	}{
		{"2026-08-02", 0, 200},
		{"2026-08-09", 0, 198},
		{"2026-08-16", 1, 196},
		{"2026-08-23", 1, 194},
		{"2026-08-30", 2, 192},
		{"2026-09-06", 2, 190},
		{"2026-09-13", 3, 188},
		{"2026-09-20", 3, 186},
		{"2026-09-27", 3, 184},
	}
	for _, row := range sundays {
		v := row.lb
		ev := aims.Event{What: "week", Day: row.day, Metric: "weight", Value: &v, Unit: "lb"}
		if _, err := f.store.Log(ctx, ev, map[string]int{"training": row.score, "weight": row.score}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.store.SetBlock(ctx, "training", "2026-08-02", "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	sunday := time.Date(2026, 10, 4, 8, 0, 0, 0, loc)
	text := f.store.ProgressText(ctx, []string{"training", "weight"}, sunday)
	for _, part := range []string{"weeks:", "slope", "r=", "block "} {
		if !strings.Contains(text, part) {
			t.Fatalf("sunday missing %q:\n%s", part, text)
		}
	}
	wednesday := time.Date(2026, 10, 7, 8, 0, 0, 0, loc)
	text = f.store.ProgressText(ctx, []string{"training", "weight"}, wednesday)
	if !strings.Contains(text, "block ") {
		t.Fatalf("wednesday block:\n%s", text)
	}
	for _, part := range []string{"weeks:", "slope", "r=", "too early"} {
		if strings.Contains(text, part) {
			t.Fatalf("wednesday has %q:\n%s", part, text)
		}
	}
	short := openFixture(t, loc)
	seedAims(t, short.mem, "training")
	for _, day := range []string{"2026-08-23", "2026-08-30", "2026-09-06", "2026-09-13", "2026-09-20", "2026-09-27", "2026-10-04"} {
		if _, err := short.store.Log(ctx, aims.Event{What: "session", Day: day}, map[string]int{"training": 1}); err != nil {
			t.Fatal(err)
		}
	}
	text = short.store.ProgressText(ctx, []string{"training"}, sunday)
	if !strings.Contains(text, "weeks:") || strings.Contains(text, "r=") || strings.Contains(text, "too early") {
		t.Fatalf("7 weeks:\n%s", text)
	}
	area := f.store.AreaTrend(ctx, "weight", wednesday)
	if !strings.Contains(area, "weeks:") {
		t.Fatalf("area on wednesday:\n%s", area)
	}
}

func mustLA(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
