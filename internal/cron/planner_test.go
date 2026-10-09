package cron_test

import (
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
)

func TestParsePlannerAt(t *testing.T) {
	hour, minute, err := cron.ParsePlannerAt("07:10")
	if err != nil || hour != 7 || minute != 10 {
		t.Fatalf("07:10 -> %d:%d %v", hour, minute, err)
	}
	hour, minute, err = cron.ParsePlannerAt("9:30pm")
	if err != nil || hour != 21 || minute != 30 {
		t.Fatalf("9:30pm -> %d:%d %v", hour, minute, err)
	}
	hour, minute, err = cron.ParsePlannerAt("9am")
	if err != nil || hour != 9 || minute != 0 {
		t.Fatalf("9am -> %d:%d %v", hour, minute, err)
	}
	if _, _, err := cron.ParsePlannerAt("4-6"); err == nil {
		t.Fatal("a count is not a clock time")
	}
	if _, _, err := cron.ParsePlannerAt("4-6@06-21"); err == nil {
		t.Fatal("a window is not a clock time")
	}
}

func TestNextPlannerAt_OneSlot(t *testing.T) {
	loc := time.UTC
	before := time.Date(2026, 7, 29, 6, 0, 0, 0, loc)
	next := cron.NextPlannerAt(7, 10, loc, before, nil, false)
	want := time.Date(2026, 7, 29, 7, 10, 0, 0, loc).UTC()
	if !next.Equal(want) {
		t.Fatalf("before: got %s want %s", next, want)
	}
	after := time.Date(2026, 7, 29, 8, 0, 0, 0, loc)
	next = cron.NextPlannerAt(7, 10, loc, after, nil, false)
	want = time.Date(2026, 7, 30, 7, 10, 0, 0, loc).UTC()
	if !next.Equal(want) {
		t.Fatalf("after: got %s want %s", next, want)
	}
	// Moving the clock during the session does not buy a second burn today.
	running := time.Date(2026, 7, 29, 7, 15, 0, 0, loc)
	next = cron.NextPlannerAt(11, 0, loc, running, nil, true)
	want = time.Date(2026, 7, 30, 11, 0, 0, 0, loc).UTC()
	if !next.Equal(want) {
		t.Fatalf("running: got %s want %s", next, want)
	}
}

func TestAdvanceNext_DailyPlanner(t *testing.T) {
	from := time.Date(2026, 7, 29, 7, 10, 0, 0, time.UTC)
	next, newExpr, ok, err := cron.AdvanceNext(cron.KindDailyPlanner, "07:10", "UTC", from)
	if err != nil || !ok || newExpr != "" {
		t.Fatalf("ok=%v expr=%q err=%v", ok, newExpr, err)
	}
	want := time.Date(2026, 7, 30, 7, 10, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next=%s want %s", next, want)
	}
}

func TestParseSchedule_Planner(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, loc)
	p, err := cron.ParseSchedule("09:30", "planner", loc, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != cron.KindDailyPlanner || p.Expr != "09:30" {
		t.Fatalf("kind=%s expr=%s", p.Kind, p.Expr)
	}
	want := time.Date(2026, 7, 29, 9, 30, 0, 0, loc).UTC()
	if !p.NextRun.Equal(want) {
		t.Fatalf("next=%s want %s", p.NextRun, want)
	}
}

func TestDailyPlannerPrompt_OneSession(t *testing.T) {
	body := cron.DailyPlannerPrefix + cron.DefaultDailyPlannerPrompt
	for _, needle := range []string{
		"one planning session",
		"Garmin",
		"calendar",
		"mail",
		"[silent]",
		"vacation",
		"repeat=planner",
		"[aims]",
		"[wait]",
		"agree-and-stop",
		"aim_log",
		"aim_history",
		"[progress]",
		"praised",
		"note=asked",
		"note=quiet",
		"weeks:",
		"slope",
		"r=",
		"[todo]",
		"memory_subject todo/<slug>",
		"never an offer to drop it",
		"[todo] is in priority order",
		"do it now",
		"no closer",
		"[silent] stays [silent]",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("planner text missing %q", needle)
		}
	}
	if !cron.IsDailyPlannerTurn(body) {
		t.Fatal("prefix should mark a planner turn")
	}
	if cron.IsDailyPlannerTurn(cron.JobUserPrefix + "Fetch Garmin sleep") {
		t.Fatal("a scheduled job is not the daily planner")
	}
}
