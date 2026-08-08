package cron_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
)

type cronFixture struct {
	store *cron.Store
	db    *sql.DB
}

func openCronFixture(t *testing.T) cronFixture {
	t.Helper()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	return cronFixture{store: store, db: sess.DB()}
}

func TestClearStaleRunning_RecoversDueJobs(t *testing.T) {
	ctx := context.Background()
	f := openCronFixture(t)
	job, err := f.store.Schedule(ctx, "recover me", cron.Parsed{
		Kind: cron.KindOnce, Expr: "x", Timezone: "UTC",
		NextRun: time.Now().UTC().Add(-time.Minute),
	}, cron.Delivery{SessionID: "s", UserID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE cron_job SET running = 1 WHERE id = ?`, job.ID); err != nil {
		t.Fatal(err)
	}
	due, err := f.store.Due(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("stuck running job should be invisible to Due, got %d", len(due))
	}
	n, err := f.store.ClearStaleRunning(ctx)
	if err != nil || n != 1 {
		t.Fatalf("ClearStaleRunning n=%d err=%v", n, err)
	}
	due, err = f.store.Due(ctx, time.Now().UTC(), 10)
	if err != nil || len(due) != 1 || due[0].ID != job.ID {
		t.Fatalf("after clear, due=%v err=%v", due, err)
	}
}

func TestFinish_DoesNotResurrectCancelledJob(t *testing.T) {
	ctx := context.Background()
	f := openCronFixture(t)
	job, err := f.store.Schedule(ctx, "daily dig", cron.Parsed{
		Kind: cron.KindDaily, Expr: "17:00", Timezone: "UTC",
		NextRun: time.Now().UTC().Add(-time.Minute),
	}, cron.Delivery{SessionID: "s", UserID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := f.store.Claim(ctx, job.ID, time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if err := f.store.Cancel(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Finish(ctx, job, nil); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("Finish resurrected a cancelled job")
	}
	if got.Running {
		t.Fatal("cancelled job should not be running")
	}
}

func TestFinish_KeepsDisabledWhenEnabledClearedMidFlight(t *testing.T) {
	ctx := context.Background()
	f := openCronFixture(t)
	job, err := f.store.Schedule(ctx, "every hour", cron.Parsed{
		Kind: cron.KindEvery, Expr: "1h", Timezone: "UTC",
		NextRun: time.Now().UTC().Add(-time.Minute),
	}, cron.Delivery{SessionID: "s", UserID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := f.store.Claim(ctx, job.ID, time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE cron_job SET enabled = 0 WHERE id = ?`, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Finish(ctx, job, nil); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("Finish must not re-enable mid-flight cancel")
	}
	if got.Running {
		t.Fatal("Finish should clear running")
	}
}

func TestCancel_SparkPlannerCascadesPings(t *testing.T) {
	ctx := context.Background()
	f := openCronFixture(t)
	delivery := cron.Delivery{SessionID: "spark-s", UserID: "u", ChatID: "1"}
	// Full-day window so EnsureSpark still seeds when CI runs after 21:00 UTC.
	parsed, err := cron.ParseSchedule("4-6@00-24", "spark", time.UTC, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	planner, _, err := f.store.EnsureSpark(ctx, "check in", parsed, delivery)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := f.store.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	pings := 0
	for _, j := range jobs {
		if j.Kind == cron.KindSparkPing {
			pings++
		}
	}
	if pings == 0 {
		t.Fatal("expected spark_ping jobs after EnsureSpark")
	}
	if err := f.store.Cancel(ctx, planner.ID); err != nil {
		t.Fatal(err)
	}
	jobs, err = f.store.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.Kind == cron.KindSpark || j.Kind == cron.KindSparkPing {
			t.Fatalf("cascade cancel left enabled %s id=%d", j.Kind, j.ID)
		}
	}
}

func TestDue_FixedWidthNanosOrdering(t *testing.T) {
	ctx := context.Background()
	f := openCronFixture(t)
	job, err := f.store.Schedule(ctx, "align", cron.Parsed{
		Kind: cron.KindOnce, Expr: "x", Timezone: "UTC",
		NextRun: time.Date(2020, 1, 1, 10, 0, 0, 0, time.UTC),
	}, cron.Delivery{SessionID: "s", UserID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2020, 1, 1, 10, 0, 0, 500000000, time.UTC)
	due, err := f.store.Due(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != job.ID {
		t.Fatalf("fixed-width compare should see job due, got %#v", due)
	}
}
