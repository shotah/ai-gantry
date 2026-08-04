package cron_test

import (
	"context"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestEnsureSpark_DoesNotCompoundOnRestart(t *testing.T) {
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
	now := time.Now().In(loc)
	parsed, err := cron.ParseSparkSchedule("4-4@06-21", 6, 21, loc, now)
	if err != nil {
		t.Fatal(err)
	}
	delivery := cron.Delivery{SessionID: "telegram:1:1", UserID: "1", ChatID: "1"}

	job1, created, err := store.EnsureSpark(ctx, "spark prompt", parsed, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected created=true on first ensure")
	}
	first := countKind(t, store, cron.KindSparkPing)
	if first < 1 {
		t.Fatalf("expected spark_ping jobs, got %d", first)
	}

	// Simulate all of today's pings having already fired.
	n, err := store.CancelSparkPings(ctx, delivery.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatal("expected to cancel pending pings")
	}

	// Reboot path: EnsureSpark again must not roll a second set for the same day.
	job2, created, err := store.EnsureSpark(ctx, "spark prompt", parsed, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected created=false on second ensure")
	}
	if job2.ID != job1.ID {
		t.Fatalf("planner id changed: %d → %d", job1.ID, job2.ID)
	}
	second := countKind(t, store, cron.KindSparkPing)
	if second != 0 {
		t.Fatalf("restart reseeded pings: want 0 pending, got %d (compounding bug)", second)
	}
	planners := countKind(t, store, cron.KindSpark)
	if planners != 1 {
		t.Fatalf("want 1 spark planner, got %d", planners)
	}
}

func TestEnsureSpark_PrunesStalePingsFromPriorDay(t *testing.T) {
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
	now := time.Now().In(loc)
	parsed, err := cron.ParseSparkSchedule("2-2@06-21", 6, 21, loc, now)
	if err != nil {
		t.Fatal(err)
	}
	delivery := cron.Delivery{SessionID: "telegram:2:2", UserID: "2", ChatID: "2"}

	if _, _, err := store.EnsureSpark(ctx, "spark prompt", parsed, delivery); err != nil {
		t.Fatal(err)
	}

	// Inject a "yesterday" leftover that would otherwise still be enabled.
	stale := time.Now().UTC().Add(-24 * time.Hour)
	if _, err := store.Schedule(ctx, "spark prompt", cron.SparkPingParsed(stale, "UTC"), delivery); err != nil {
		t.Fatal(err)
	}
	before := countKind(t, store, cron.KindSparkPing)
	if before < 2 {
		t.Fatalf("setup: want today pings + stale, got %d", before)
	}

	if _, _, err := store.EnsureSpark(ctx, "spark prompt", parsed, delivery); err != nil {
		t.Fatal(err)
	}
	after := countKind(t, store, cron.KindSparkPing)
	// Stale gone; today's future pings kept (no full reseed).
	if after != before-1 {
		t.Fatalf("stale prune: before=%d after=%d want %d", before, after, before-1)
	}
	jobs, err := store.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.Kind == cron.KindSparkPing && !j.NextRunAt.After(stale) {
			t.Fatalf("stale ping still enabled: id=%d next=%s", j.ID, j.NextRunAt)
		}
	}
}

func TestEnsureSpark_DisablesDuplicatePlanners(t *testing.T) {
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
	parsed, err := cron.ParseSparkSchedule("3-3@06-21", 6, 21, loc, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	delivery := cron.Delivery{SessionID: "telegram:3:3", UserID: "3", ChatID: "3"}

	job, _, err := store.EnsureSpark(ctx, "spark prompt", parsed, delivery)
	if err != nil {
		t.Fatal(err)
	}
	// Bypass EnsureSpark (legacy / bug path) and insert a second planner.
	dup := parsed
	dup.NextRun = time.Now().UTC().Add(2 * time.Hour)
	if _, err := store.Schedule(ctx, "spark prompt", dup, delivery); err != nil {
		t.Fatal(err)
	}
	if n := countKind(t, store, cron.KindSpark); n != 2 {
		t.Fatalf("setup planners=%d", n)
	}

	kept, _, err := store.EnsureSpark(ctx, "spark prompt", parsed, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if n := countKind(t, store, cron.KindSpark); n != 1 {
		t.Fatalf("want 1 planner after ensure, got %d", n)
	}
	// Keep the newest FindSpark row (ORDER BY id DESC); duplicate older ones drop.
	if kept.ID < job.ID {
		t.Fatalf("expected keep newest planner, got id=%d first=%d", kept.ID, job.ID)
	}
}

func TestRunner_SparkPlannerCancelsBeforeReseed(t *testing.T) {
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

	delivery := cron.Delivery{SessionID: "telegram:4:4", UserID: "4", ChatID: "4"}
	past := time.Now().UTC().Add(-time.Minute)
	if _, err := store.Schedule(ctx, "spark prompt", cron.Parsed{
		Kind:     cron.KindSpark,
		Expr:     "2-2@06-21",
		NextRun:  past,
		Timezone: "UTC",
	}, delivery); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Schedule(ctx, "spark prompt", cron.SparkPingParsed(past, "UTC"), delivery); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Schedule(ctx, "spark prompt", cron.SparkPingParsed(past.Add(-time.Hour), "UTC"), delivery); err != nil {
		t.Fatal(err)
	}

	runner := &cron.Runner{
		Store: store,
		Handle: func(context.Context, channel.Message) (string, error) {
			return "ping", nil
		},
		Pusher: &memPusher{},
	}
	runner.FireDueForTest(ctx)

	// Leftover pings cancelled; a fresh same-day set may be inserted (qty 2),
	// but we must not keep the old overdue rows in addition.
	pings := 0
	jobs, err := store.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.Kind == cron.KindSparkPing {
			pings++
			if !j.NextRunAt.After(past.Add(-30 * time.Second)) {
				t.Fatalf("old leftover ping still enabled id=%d next=%s", j.ID, j.NextRunAt)
			}
		}
	}
	if pings > 2 {
		t.Fatalf("planner compounded pings: got %d want <= 2", pings)
	}
}

func countKind(t *testing.T, store *cron.Store, kind string) int {
	t.Helper()
	jobs, err := store.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, j := range jobs {
		if j.Kind == kind {
			n++
		}
	}
	return n
}
