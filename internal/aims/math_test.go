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
}
