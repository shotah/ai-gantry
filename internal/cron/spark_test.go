package cron_test

import (
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
)

func TestParseSparkPrompts(t *testing.T) {
	defaults := cron.ParseSparkPrompts("")
	if len(defaults) < 5 {
		t.Fatalf("empty → default pool, got %d entries: %#v", len(defaults), defaults)
	}
	for _, p := range defaults {
		if strings.Contains(strings.ToLower(p), "no tools") {
			t.Fatalf("default spark prompt must allow tools: %q", p)
		}
		if !strings.Contains(p, "aim/") && !strings.Contains(p, "SELF.md") && !strings.Contains(p, "cron") {
			t.Fatalf("default spark prompt must be horizon work: %q", p)
		}
		if !strings.Contains(p, "[silent]") && !strings.Contains(p, "silent") {
			t.Fatalf("default spark prompt must mention silent: %q", p)
		}
	}
	fromConst := cron.ParseSparkPrompts(cron.DefaultSparkPrompt)
	if len(fromConst) != len(defaults) {
		t.Fatalf("DefaultSparkPrompt parse len=%d, empty parse len=%d", len(fromConst), len(defaults))
	}
	if !strings.Contains(cron.SparkPingPrefix, "aim/") {
		t.Fatal("spark prefix must mention aim/")
	}
	if strings.Contains(cron.SparkPingPrefix, "Do not send a joke") {
		t.Fatal("spark prefix must not blanket-ban jokes")
	}
	if !strings.Contains(strings.ToLower(cron.SparkPingPrefix), "joke") {
		t.Fatal("spark prefix should allow a grounded joke")
	}
	var sawFood, sawGarmin, sawHours bool
	for _, p := range defaults {
		if strings.Contains(p, "pref/food") {
			sawFood = true
		}
		if strings.Contains(strings.ToLower(p), "garmin") {
			sawGarmin = true
		}
		if strings.Contains(p, "pref/hours") {
			sawHours = true
		}
	}
	if !sawFood || !sawGarmin || !sawHours {
		t.Fatalf("pool missing user-model/gym/hours lines food=%v garmin=%v hours=%v", sawFood, sawGarmin, sawHours)
	}
	// commas/colons stay inside a single prompt
	single := cron.ParseSparkPrompts("Tell a joke, then smile: briefly.")
	if len(single) != 1 || single[0] != "Tell a joke, then smile: briefly." {
		t.Fatalf("got %#v", single)
	}
	pool := cron.ParseSparkPrompts("joke A\njoke B\n")
	if len(pool) != 2 || pool[0] != "joke A" || pool[1] != "joke B" {
		t.Fatalf("got %#v", pool)
	}
	esc := cron.ParseSparkPrompts(`a\nb\nc`)
	if len(esc) != 3 {
		t.Fatalf("escaped \\n pool: %#v", esc)
	}
	picked := map[string]bool{}
	for i := 0; i < 40; i++ {
		picked[cron.PickSparkPrompt("a\nb\nc")] = true
	}
	if len(picked) < 2 {
		t.Fatalf("expected variety from pool, got %v", picked)
	}
}

func TestParseSparkQty(t *testing.T) {
	qtyMin, qtyMax, err := cron.ParseSparkQty("4-6")
	if err != nil || qtyMin != 4 || qtyMax != 6 {
		t.Fatalf("got %d-%d err=%v", qtyMin, qtyMax, err)
	}
	qtyMin, qtyMax, err = cron.ParseSparkQty("5")
	if err != nil || qtyMin != 5 || qtyMax != 5 {
		t.Fatalf("got %d-%d err=%v", qtyMin, qtyMax, err)
	}
	if _, _, err := cron.ParseSparkQty(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestPlanSparkDayTimes_FullWindow(t *testing.T) {
	loc := time.UTC
	// At window start: expect a full qty roll, all times inside [06,21).
	now := time.Date(2026, 7, 29, 6, 0, 0, 0, loc)
	spec := cron.SparkSpec{Min: 4, Max: 6, StartHour: 6, EndHour: 21}
	n, times, err := cron.PlanSparkDayTimes(spec, loc, now)
	if err != nil {
		t.Fatal(err)
	}
	if n < 4 || n > 6 {
		t.Fatalf("qty=%d want 4–6", n)
	}
	if len(times) != n {
		t.Fatalf("len(times)=%d want %d", len(times), n)
	}
	end := time.Date(2026, 7, 29, 21, 0, 0, 0, loc)
	for i, tm := range times {
		local := tm.In(loc)
		if local.Before(now) || !local.Before(end) {
			t.Fatalf("times[%d]=%s outside window", i, local)
		}
		if i > 0 && !tm.After(times[i-1]) {
			t.Fatalf("times not strictly increasing: %v", times)
		}
	}
}

func TestPlanSparkDayTimes_RemainingWindow(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 29, 18, 0, 0, 0, loc)
	spec := cron.SparkSpec{Min: 4, Max: 4, StartHour: 6, EndHour: 21}
	n, times, err := cron.PlanSparkDayTimes(spec, loc, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 || len(times) != 4 {
		t.Fatalf("qty=%d times=%d", n, len(times))
	}
	end := time.Date(2026, 7, 29, 21, 0, 0, 0, loc)
	for _, tm := range times {
		if !tm.After(now) || !tm.In(loc).Before(end) {
			t.Fatalf("time %s not in remaining window", tm)
		}
	}
}

func TestPlanSparkPlannerNext(t *testing.T) {
	loc := time.UTC
	spec := cron.SparkSpec{Min: 4, Max: 6, StartHour: 6, EndHour: 21}

	before := time.Date(2026, 7, 29, 5, 0, 0, 0, loc)
	next := cron.PlanSparkPlannerNext(spec, loc, before)
	want := time.Date(2026, 7, 29, 6, 0, 0, 0, loc).UTC()
	if !next.Equal(want) {
		t.Fatalf("before window: got %s want %s", next, want)
	}

	mid := time.Date(2026, 7, 29, 12, 0, 0, 0, loc)
	next = cron.PlanSparkPlannerNext(spec, loc, mid)
	want = time.Date(2026, 7, 30, 6, 0, 0, 0, loc).UTC()
	if !next.Equal(want) {
		t.Fatalf("mid day: got %s want %s", next, want)
	}
}

func TestAdvanceNext_Spark(t *testing.T) {
	from := time.Date(2026, 7, 29, 6, 0, 0, 0, time.UTC)
	next, newExpr, ok, err := cron.AdvanceNext(cron.KindSpark, "4-6@06-21", "UTC", from)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if newExpr != "" {
		t.Fatalf("newExpr=%q want empty (template unchanged)", newExpr)
	}
	want := time.Date(2026, 7, 30, 6, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next=%s want %s", next, want)
	}

	_, _, ok, err = cron.AdvanceNext(cron.KindSparkPing, from.Format(time.RFC3339Nano), "UTC", from)
	if err != nil || ok {
		t.Fatalf("spark_ping should finish: ok=%v err=%v", ok, err)
	}
}

func TestParseSchedule_Spark(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, loc)
	p, err := cron.ParseSchedule("4-6@06-21", "spark", loc, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != cron.KindSpark {
		t.Fatalf("kind=%s", p.Kind)
	}
	if p.Expr != "4-6@06-21" {
		t.Fatalf("expr=%q", p.Expr)
	}
}

func TestIsSparkTurn(t *testing.T) {
	t.Parallel()
	if !cron.IsSparkTurn(cron.SparkPingPrefix + "recall aim/") {
		t.Fatal("prefix + body should be a spark turn")
	}
	if cron.IsSparkTurn(cron.JobUserPrefix + "Fetch Garmin sleep") {
		t.Fatal("scheduled job is not a spark turn")
	}
	if cron.IsSparkTurn("hey") {
		t.Fatal("user chat is not a spark turn")
	}
}
