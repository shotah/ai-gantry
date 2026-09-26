package cron_test

import (
	"context"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestPlannerService_ClockAndOptOut(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	svc := &cron.PlannerService{Store: store, TZ: "UTC", At: "07:10"}
	delivery := cron.Delivery{SessionID: channel.AgentSession}

	got, err := svc.ResolvedAt(ctx, delivery.SessionID)
	if err != nil || got != "07:10" {
		t.Fatalf("inherit: %q %v", got, err)
	}
	if err := svc.SetAt(ctx, delivery.SessionID, "9:30am"); err != nil {
		t.Fatal(err)
	}
	job, _, err := svc.EnsureFor(ctx, delivery)
	if err != nil || job.ID == 0 || job.Expr != "09:30" || job.Kind != cron.KindDailyPlanner {
		t.Fatalf("job id=%d kind=%s expr=%s err=%v", job.ID, job.Kind, job.Expr, err)
	}
	if err := svc.SetAt(ctx, delivery.SessionID, "3-5"); err == nil {
		t.Fatal("a count is not a planner time")
	}
	if err := svc.SetAt(ctx, delivery.SessionID, cron.PlannerOff); err != nil {
		t.Fatal(err)
	}
	job, _, err = svc.EnsureFor(ctx, delivery)
	if err != nil || job.ID != 0 {
		t.Fatalf("opt out id=%d err=%v", job.ID, err)
	}
	if _, ok, err := store.FindDailyPlanner(ctx, delivery.SessionID); err != nil || ok {
		t.Fatalf("planner still on ok=%v err=%v", ok, err)
	}
}

func TestEnsureDailyPlanner_OneJob(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	loc := time.UTC
	parsed, err := cron.ParsePlannerSchedule("07:10", loc, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	delivery := cron.Delivery{SessionID: channel.AgentSession}
	job1, created, err := store.EnsureDailyPlanner(ctx, cron.DefaultDailyPlannerPrompt, parsed, delivery)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	job2, created, err := store.EnsureDailyPlanner(ctx, cron.DefaultDailyPlannerPrompt, parsed, delivery)
	if err != nil || created || job2.ID != job1.ID {
		t.Fatalf("second id=%d created=%v err=%v", job2.ID, created, err)
	}
	jobs, err := store.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, j := range jobs {
		if j.Kind == cron.KindDailyPlanner {
			n++
		}
		if j.Kind != cron.KindDailyPlanner {
			t.Fatalf("planner seeded an extra job kind=%s", j.Kind)
		}
	}
	if n != 1 {
		t.Fatalf("planners=%d", n)
	}
}

func TestFinish_KeepsClockMovedDuringRun(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	delivery := cron.Delivery{SessionID: channel.AgentSession}
	job, err := store.Schedule(ctx, cron.DefaultDailyPlannerPrompt, cron.Parsed{
		Kind: cron.KindDailyPlanner, Expr: "07:10", NextRun: past, Timezone: "UTC",
	}, delivery)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := store.Claim(ctx, job.ID, time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	moved, err := cron.ParsePlannerSchedule("11:00", time.UTC, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EnsureDailyPlanner(ctx, cron.DefaultDailyPlannerPrompt, moved, delivery); err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, job, nil); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Running {
		t.Fatal("finish should clear running")
	}
	if got.Expr != "11:00" {
		t.Fatalf("expr=%s want 11:00", got.Expr)
	}
	if got.NextRunAt.In(time.UTC).Hour() != 11 {
		t.Fatalf("next=%s want 11:00", got.NextRunAt.UTC())
	}
}
