package cron_test

import (
	"context"
	"testing"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestSparkService_QtyOverrideAndOptOut(t *testing.T) {
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
	svc := &cron.SparkService{
		Store:     store,
		StartHour: 0,
		EndHour:   24,
		TZ:        "UTC",
		Prompt:    "spark prompt",
	}
	delivery := cron.Delivery{SessionID: "telegram:8:8", UserID: "8", ChatID: "8"}

	got, err := svc.ResolvedQty(ctx, delivery.SessionID)
	if err != nil || got != "3-5" {
		t.Fatalf("inherit: %q %v", got, err)
	}

	if err := svc.SetQty(ctx, delivery.SessionID, "4"); err != nil {
		t.Fatal(err)
	}
	got, err = svc.ResolvedQty(ctx, delivery.SessionID)
	if err != nil || got != "4" {
		t.Fatalf("override: %q %v", got, err)
	}
	job, _, err := svc.EnsureFor(ctx, delivery)
	if err != nil || job.ID == 0 {
		t.Fatalf("ensure override: id=%d err=%v", job.ID, err)
	}
	if job.Expr != "4-4@00-24" {
		t.Fatalf("expr=%q", job.Expr)
	}

	if err := svc.SetQty(ctx, delivery.SessionID, "0"); err != nil {
		t.Fatal(err)
	}
	job, _, err = svc.EnsureFor(ctx, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != 0 {
		t.Fatalf("opt out should skip ensure, got id=%d", job.ID)
	}
	if n := countKind(t, store, cron.KindSpark); n != 0 {
		t.Fatalf("planner still enabled: %d", n)
	}
	if n := countKind(t, store, cron.KindSparkPing); n != 0 {
		t.Fatalf("pings still enabled: %d", n)
	}
}
