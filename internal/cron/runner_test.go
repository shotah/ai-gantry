package cron_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
)

type memPusher struct {
	mu   sync.Mutex
	msgs []channel.Outbound
}

func (m *memPusher) Push(_ context.Context, msg channel.Outbound) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
	return nil
}

func TestRunner_ScheduleFirePushCancel(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	store, err := cron.OpenDB(sess.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}

	delivery := cron.Delivery{
		SessionID: "telegram:1:2",
		UserID:    "2",
		ChatID:    "1",
	}
	past := time.Now().UTC().Add(-time.Minute)
	job, err := store.Schedule(ctx, "submit timecard", cron.Parsed{
		Kind:     cron.KindOnce,
		Expr:     past.Format(time.RFC3339Nano),
		NextRun:  past,
		Timezone: "UTC",
	}, delivery)
	if err != nil {
		t.Fatal(err)
	}

	pusher := &memPusher{}
	var handled string
	runner := &cron.Runner{
		Store: store,
		Handle: func(_ context.Context, msg channel.Message) (string, error) {
			handled = msg.Text
			return "timecard reminder: do it now", nil
		},
		Pusher: pusher,
	}
	runner.FireDueForTest(ctx)

	if handled == "" || handled[:6] != "[cron]" {
		t.Fatalf("handle text=%q", handled)
	}
	pusher.mu.Lock()
	n := len(pusher.msgs)
	pusher.mu.Unlock()
	if n != 1 {
		t.Fatalf("pushes=%d", n)
	}

	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("once job should be disabled after fire")
	}

	// cancel path via tools
	tools := cron.Tools{Store: store, TZ: "UTC"}
	ctx = cron.WithDelivery(ctx, delivery)
	_, err = tools.Call(ctx, cron.ToolSchedule, []byte(`{"prompt":"hi","when":"in 1h"}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := tools.Call(ctx, cron.ToolList, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "no active cron jobs" {
		t.Fatal(out)
	}
}

func TestRunner_StartAndNil(t *testing.T) {
	(&cron.Runner{}).Start(context.Background()) // no-op

	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	_, err = store.Schedule(ctx, "x", cron.Parsed{
		Kind: cron.KindOnce, Expr: past.Format(time.RFC3339Nano), NextRun: past, Timezone: "UTC",
	}, cron.Delivery{SessionID: "s", UserID: "u", ChatID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	pusher := &memPusher{}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		(&cron.Runner{
			Store:    store,
			Handle:   func(context.Context, channel.Message) (string, error) { return "ok", nil },
			Pusher:   pusher,
			Interval: 15 * time.Millisecond,
		}).Start(runCtx)
		close(done)
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done
	pusher.mu.Lock()
	n := len(pusher.msgs)
	pusher.mu.Unlock()
	if n < 1 {
		t.Fatal("expected push from Start poll")
	}
}
