package examples_test

import (
	"context"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/examples"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestService_ProactiveAndEnsureRespectsOptOut(t *testing.T) {
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

	svc := &examples.Service{
		Store:     store,
		Qty:       "1-1",
		StartHour: 0,
		EndHour:   24,
		TZ:        "UTC",
		Tools: func() []provider.ToolDef {
			return []provider.ToolDef{{Name: "google__calendar_list_events"}}
		},
	}
	if !svc.ProactiveEnabled() {
		t.Fatal("want proactive on")
	}
	off := &examples.Service{Qty: "0"}
	if off.ProactiveEnabled() {
		t.Fatal("qty 0 should disable")
	}
	empty := &examples.Service{Qty: ""}
	if empty.ProactiveEnabled() {
		t.Fatal("empty qty should disable")
	}

	delivery := cron.Delivery{SessionID: "telegram:99:99", UserID: "99", ChatID: "99"}
	job, created, err := svc.EnsureFor(ctx, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if !created || job.ID == 0 {
		t.Fatalf("ensure: created=%v id=%d", created, job.ID)
	}

	prompt, seed, ok := svc.SuggestPrompt()
	if !ok || prompt == "" || seed.ID == "" {
		t.Fatalf("suggest: ok=%v seed=%+v", ok, seed)
	}

	if err := svc.SetEnabled(ctx, delivery.SessionID, false); err != nil {
		t.Fatal(err)
	}
	job2, _, err := svc.EnsureFor(ctx, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if job2.ID != 0 {
		t.Fatal("ensure after opt-out should no-op")
	}

	// Allow planner wake next day path not needed; just restore.
	if err := svc.SetEnabled(ctx, delivery.SessionID, true); err != nil {
		t.Fatal(err)
	}
	// Re-ensure after cancel: planner was cancelled by SetEnabled(false).
	time.Sleep(0)
	job3, created3, err := svc.EnsureFor(ctx, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if job3.ID == 0 {
		t.Fatal("ensure after on should create planner")
	}
	_ = created3
}
