package cron_test

import (
	"context"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestEnsureExamples_DoesNotCompoundOnRestart(t *testing.T) {
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
	parsed, err := cron.ParseExamplesSchedule("2-2@00-24", 0, 24, loc, now)
	if err != nil {
		t.Fatal(err)
	}
	delivery := cron.Delivery{SessionID: "telegram:11:11", UserID: "11", ChatID: "11"}

	job1, created, err := store.EnsureExamples(ctx, "examples prompt", parsed, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected created=true on first ensure")
	}
	first := countKind(t, store, cron.KindExamplesPing)
	if first < 1 {
		t.Fatalf("expected examples_ping jobs, got %d", first)
	}

	n, err := store.CancelExamplesPings(ctx, delivery.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatal("expected to cancel pending pings")
	}

	job2, created, err := store.EnsureExamples(ctx, "examples prompt", parsed, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected created=false on second ensure")
	}
	if job2.ID != job1.ID {
		t.Fatalf("planner id changed: %d → %d", job1.ID, job2.ID)
	}
	second := countKind(t, store, cron.KindExamplesPing)
	if second != 0 {
		t.Fatalf("restart reseeded pings: want 0 pending, got %d", second)
	}
}

func TestExamplesPref_OptOutSurvivesEnsure(t *testing.T) {
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

	delivery := cron.Delivery{SessionID: "telegram:12:12", UserID: "12", ChatID: "12"}
	on, err := store.ExamplesEnabled(ctx, delivery.SessionID)
	if err != nil || !on {
		t.Fatalf("default enabled: on=%v err=%v", on, err)
	}

	loc := time.UTC
	parsed, err := cron.ParseExamplesSchedule("1-1@00-24", 0, 24, loc, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EnsureExamples(ctx, "p", parsed, delivery); err != nil {
		t.Fatal(err)
	}
	if countKind(t, store, cron.KindExamples) != 1 {
		t.Fatal("want planner")
	}

	if err := store.SetExamplesEnabled(ctx, delivery.SessionID, false); err != nil {
		t.Fatal(err)
	}
	n, err := store.CancelExamplesPlannerAndPings(ctx, delivery.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatal("expected cancel")
	}
	on, err = store.ExamplesEnabled(ctx, delivery.SessionID)
	if err != nil || on {
		t.Fatalf("want opted out: on=%v err=%v", on, err)
	}

	// Boot-style ensure must not recreate when caller respects pref.
	on, err = store.ExamplesEnabled(ctx, delivery.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatal("pref should still be off")
	}
	if countKind(t, store, cron.KindExamples) != 0 {
		t.Fatal("planner should stay cancelled")
	}

	if err := store.SetExamplesEnabled(ctx, delivery.SessionID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EnsureExamples(ctx, "p", parsed, delivery); err != nil {
		t.Fatal(err)
	}
	if countKind(t, store, cron.KindExamples) != 1 {
		t.Fatal("want planner after on")
	}
}

func TestRunner_ExamplesPingUsesNoTools(t *testing.T) {
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
	delivery := cron.Delivery{SessionID: "telegram:13:13", UserID: "13", ChatID: "13"}
	_, err = store.Schedule(ctx, "placeholder", cron.ExamplesPingParsed(past, "UTC"), delivery)
	if err != nil {
		t.Fatal(err)
	}

	var sawNoTools bool
	var prompt string
	runner := &cron.Runner{
		Store: store,
		Examples: pingBuilderFunc(func(_ context.Context) string {
			return "polish this seed"
		}),
		Handle: func(ctx context.Context, msg channel.Message) (string, error) {
			sawNoTools = channel.NoToolsFrom(ctx)
			prompt = msg.Text
			return "try this calendar idea", nil
		},
		Pusher: &memPusher{},
	}
	runner.FireDueForTest(ctx)

	if !sawNoTools {
		t.Fatal("examples_ping must use WithNoTools")
	}
	if prompt == "" || prompt[:6] != "[cron]" {
		t.Fatalf("prompt=%q", prompt)
	}
}

type pingBuilderFunc func(context.Context) string

func (f pingBuilderFunc) BuildPingPrompt(ctx context.Context) string { return f(ctx) }
