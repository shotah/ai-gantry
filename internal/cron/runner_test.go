package cron_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/memory"
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
	if !strings.Contains(handled, "call those tools first") || !strings.Contains(handled, "submit timecard") {
		t.Fatalf("handle text missing tool-first prefix or job body: %q", handled)
	}
	pusher.mu.Lock()
	n := len(pusher.msgs)
	frameID := ""
	if n == 1 {
		frameID = pusher.msgs[0].ID
	}
	pusher.mu.Unlock()
	if n != 1 {
		t.Fatalf("pushes=%d", n)
	}
	if !strings.HasPrefix(frameID, "cron-") {
		t.Fatalf("frame id %q", frameID)
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
	if out == "no cron jobs" {
		t.Fatal(out)
	}
}

func TestRunner_PushesMCPPhotos(t *testing.T) {
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
	past := time.Now().UTC().Add(-time.Minute)
	_, err = store.Schedule(ctx, "draw the bike", cron.Parsed{
		Kind:     cron.KindOnce,
		Expr:     past.Format(time.RFC3339Nano),
		NextRun:  past,
		Timezone: "UTC",
	}, cron.Delivery{SessionID: "telegram:1:2"})
	if err != nil {
		t.Fatal(err)
	}

	pusher := &memPusher{}
	runner := &cron.Runner{
		Store: store,
		Handle: func(handleCtx context.Context, _ channel.Message) (string, error) {
			if s := channel.PhotoSinkFrom(handleCtx); s != nil {
				s.Add("data:image/png;base64,AQID")
			}
			return "drew it", nil
		},
		Pusher: pusher,
	}
	runner.FireDueForTest(ctx)

	pusher.mu.Lock()
	defer pusher.mu.Unlock()
	if len(pusher.msgs) != 1 {
		t.Fatalf("pushes=%d", len(pusher.msgs))
	}
	if len(pusher.msgs[0].Photos) != 1 || !strings.HasPrefix(pusher.msgs[0].Photos[0], "data:image/png") {
		t.Fatalf("photos=%v", pusher.msgs[0].Photos)
	}
}

func TestRunner_SilentReplySkipsPush(t *testing.T) {
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
	past := time.Now().UTC().Add(-time.Minute)
	job, err := store.Schedule(ctx, "dead-man: check garmin, stay quiet if fine", cron.Parsed{
		Kind:     cron.KindOnce,
		Expr:     past.Format(time.RFC3339Nano),
		NextRun:  past,
		Timezone: "UTC",
	}, cron.Delivery{SessionID: "telegram:1:2", UserID: "2", ChatID: "1"})
	if err != nil {
		t.Fatal(err)
	}

	pusher := &memPusher{}
	var handled string
	runner := &cron.Runner{
		Store: store,
		Handle: func(_ context.Context, msg channel.Message) (string, error) {
			handled = msg.Text
			return cron.SilentToken + "\nall-clear", nil
		},
		Pusher: pusher,
	}
	runner.FireDueForTest(ctx)

	if !strings.Contains(handled, "[silent]") {
		t.Fatalf("job prefix should mention [silent]: %q", handled)
	}
	pusher.mu.Lock()
	n := len(pusher.msgs)
	pusher.mu.Unlock()
	if n != 0 {
		t.Fatalf("silent reply must not push, got %d", n)
	}
	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("once job should finish (disable) after silent skip")
	}
}

func TestRunner_SparkPingAllowsTools(t *testing.T) {
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
	past := time.Now().UTC().Add(-time.Minute)
	_, err = store.Schedule(ctx, cron.DefaultSparkPrompt, cron.SparkPingParsed(past, "UTC"), cron.Delivery{
		SessionID: "telegram:1:2", UserID: "2", ChatID: "1",
	})
	if err != nil {
		t.Fatal(err)
	}

	pusher := &memPusher{}
	var handled string
	var sawNoTools bool
	runner := &cron.Runner{
		Store: store,
		Handle: func(ctx context.Context, msg channel.Message) (string, error) {
			sawNoTools = channel.NoToolsFrom(ctx)
			handled = msg.Text
			return cron.SilentToken, nil
		},
		Pusher: pusher,
	}
	runner.FireDueForTest(ctx)

	if sawNoTools {
		t.Fatal("spark_ping must allow tools")
	}
	if !cron.IsSparkTurn(handled) {
		t.Fatalf("prompt=%q", handled)
	}
	if !strings.Contains(handled, "aim/") && !strings.Contains(handled, "SELF.md") {
		t.Fatalf("spark prompt should mention aims: %q", handled)
	}
	pusher.mu.Lock()
	n := len(pusher.msgs)
	pusher.mu.Unlock()
	if n != 0 {
		t.Fatalf("silent spark must not push, got %d", n)
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

func TestRunner_JobMemoryInjectAndSleepSkip(t *testing.T) {
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
	mem, err := memory.OpenDB(sess.DB())
	if err != nil {
		t.Fatal(err)
	}

	row, err := mem.Store(ctx, memory.KindFact, "follow/passport", "Renew next month; offer to book.")
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	_, err = store.ScheduleWithPin(ctx, "check the passport loop", cron.Parsed{
		Kind: cron.KindOnce, Expr: past.Format(time.RFC3339Nano), NextRun: past, Timezone: "UTC",
	}, cron.Delivery{SessionID: "telegram:1:2", UserID: "2", ChatID: "1"}, row.ID, row.Subject)
	if err != nil {
		t.Fatal(err)
	}

	var handled string
	runner := &cron.Runner{
		Store:  store,
		Memory: mem,
		Handle: func(_ context.Context, msg channel.Message) (string, error) {
			handled = msg.Text
			return "ok", nil
		},
		Pusher: &memPusher{},
	}
	runner.FireDueForTest(ctx)
	if !strings.Contains(handled, "[job memory]") || !strings.Contains(handled, "follow/passport") {
		t.Fatalf("missing pinned memory: %q", handled)
	}
	if !strings.Contains(handled, "check the passport loop") {
		t.Fatalf("missing job body: %q", handled)
	}

	if _, err := mem.Store(ctx, memory.KindPreference, memory.SubjectHours, "sleep: 00:00-23:59\nwork: 09:00-17:00\n"); err != nil {
		t.Fatal(err)
	}
	_, err = store.Schedule(ctx, cron.DefaultSparkPrompt, cron.SparkPingParsed(time.Now().UTC().Add(-time.Minute), "UTC"), cron.Delivery{
		SessionID: "telegram:1:9", UserID: "9", ChatID: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	var sparkHandled bool
	sleepRunner := &cron.Runner{
		Store:  store,
		Memory: mem,
		Handle: func(context.Context, channel.Message) (string, error) {
			sparkHandled = true
			return cron.SilentToken, nil
		},
		Pusher: &memPusher{},
	}
	sleepRunner.FireDueForTest(ctx)
	if sparkHandled {
		t.Fatal("spark ping should defer during sleep hours")
	}
}

func TestRunner_JobMemorySupersedeWalkAndMissingStillRuns(t *testing.T) {
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
	mem, err := memory.OpenDB(sess.DB())
	if err != nil {
		t.Fatal(err)
	}

	old, err := mem.Store(ctx, memory.KindFact, "follow/passport", "needs renewal — old note")
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	_, err = store.ScheduleWithPin(ctx, "check the passport loop", cron.Parsed{
		Kind: cron.KindOnce, Expr: past.Format(time.RFC3339Nano), NextRun: past, Timezone: "UTC",
	}, cron.Delivery{SessionID: "telegram:1:3", UserID: "3", ChatID: "1"}, old.ID, old.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindFact, "follow/passport", "booked the appointment"); err != nil {
		t.Fatal(err)
	}

	var handled string
	runner := &cron.Runner{
		Store:  store,
		Memory: mem,
		Handle: func(_ context.Context, msg channel.Message) (string, error) {
			handled = msg.Text
			return "ok", nil
		},
		Pusher: &memPusher{},
	}
	runner.FireDueForTest(ctx)
	if !strings.Contains(handled, "booked the appointment") {
		t.Fatalf("wake should walk supersede to live row: %q", handled)
	}
	if strings.Contains(handled, "old note") {
		t.Fatalf("wake still has superseded content: %q", handled)
	}

	gone, err := mem.Store(ctx, memory.KindFact, "follow/dentist", "call the office")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ScheduleWithPin(ctx, "dentist follow-up", cron.Parsed{
		Kind: cron.KindOnce, Expr: past.Format(time.RFC3339Nano), NextRun: past, Timezone: "UTC",
	}, cron.Delivery{SessionID: "telegram:1:4", UserID: "4", ChatID: "1"}, gone.ID, gone.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.Forget(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}
	handled = ""
	var ran bool
	missing := &cron.Runner{
		Store:  store,
		Memory: mem,
		Handle: func(_ context.Context, msg channel.Message) (string, error) {
			ran = true
			handled = msg.Text
			return "ok", nil
		},
		Pusher: &memPusher{},
	}
	missing.FireDueForTest(ctx)
	if !ran {
		t.Fatal("deleted pin must still run the job")
	}
	if strings.Contains(handled, "[job memory]\n") || strings.Contains(handled, "call the office") {
		t.Fatalf("missing row should omit pinned content: %q", handled)
	}
	if !strings.Contains(handled, "dentist follow-up") {
		t.Fatalf("missing job body: %q", handled)
	}
}

func TestRunner_MouthSwitchKeepsDailySummary(t *testing.T) {
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
	past := time.Now().UTC().Add(-time.Minute)
	job, err := store.Schedule(ctx, "morning summary", cron.Parsed{
		Kind:     cron.KindDaily,
		Expr:     "07:00",
		NextRun:  past,
		Timezone: "America/Los_Angeles",
	}, cron.Delivery{SessionID: "telegram:99:42", UserID: "42", ChatID: "99"})
	if err != nil {
		t.Fatal(err)
	}

	// CHANNEL=telegram → pendant: reopen the same DB (boot migrate).
	store, err = cron.OpenDB(sess.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Kind != cron.KindDaily {
		t.Fatalf("job dropped on mouth switch: %+v", got)
	}
	if got.SessionID != channel.AgentSession {
		t.Fatalf("session=%q", got.SessionID)
	}
	if got.NextRunAt.After(time.Now().UTC()) {
		t.Fatalf("next_run should still be due, got %v", got.NextRunAt)
	}

	pusher := &memPusher{}
	var handled channel.Message
	runner := &cron.Runner{
		Store: store,
		Handle: func(_ context.Context, msg channel.Message) (string, error) {
			handled = msg
			return "here's your morning summary", nil
		},
		Pusher: pusher,
	}
	runner.FireDueForTest(ctx)
	if handled.Text == "" || !strings.Contains(handled.Text, "morning summary") {
		t.Fatalf("handle=%q", handled.Text)
	}
	if handled.SessionID != channel.AgentSession || handled.UserID != "" || handled.ChatID != "" {
		t.Fatalf("handle dest %+v", handled)
	}
	pusher.mu.Lock()
	n := len(pusher.msgs)
	out := channel.Outbound{}
	if n == 1 {
		out = pusher.msgs[0]
	}
	pusher.mu.Unlock()
	if n != 1 || out.Text != "here's your morning summary" {
		t.Fatalf("pushes=%d out=%+v", n, out)
	}
	if out.UserID != "" || out.ChatID != "" || out.SessionID != "" {
		t.Fatalf("push must not carry telegram dest %+v", out)
	}
	got, err = store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled {
		t.Fatal("daily job must stay enabled after fire")
	}
}

func TestRunner_MouthSwitchKeepsSparkPing(t *testing.T) {
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
	past := time.Now().UTC().Add(-time.Minute)
	job, err := store.Schedule(ctx, cron.DefaultSparkPrompt, cron.SparkPingParsed(past, "UTC"), cron.Delivery{
		SessionID: "telegram:1:2", UserID: "2", ChatID: "1",
	})
	if err != nil {
		t.Fatal(err)
	}

	store, err = cron.OpenDB(sess.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Kind != cron.KindSparkPing {
		t.Fatalf("spark ping dropped on mouth switch: %+v", got)
	}
	if got.SessionID != channel.AgentSession {
		t.Fatalf("session=%q", got.SessionID)
	}

	pusher := &memPusher{}
	var handled channel.Message
	runner := &cron.Runner{
		Store: store,
		Handle: func(_ context.Context, msg channel.Message) (string, error) {
			handled = msg
			return "aim check-in", nil
		},
		Pusher: pusher,
	}
	runner.FireDueForTest(ctx)
	if !cron.IsSparkTurn(handled.Text) {
		t.Fatalf("handle=%q", handled.Text)
	}
	if handled.SessionID != channel.AgentSession {
		t.Fatalf("handle session=%q", handled.SessionID)
	}
	pusher.mu.Lock()
	n := len(pusher.msgs)
	out := channel.Outbound{}
	if n == 1 {
		out = pusher.msgs[0]
	}
	pusher.mu.Unlock()
	if n != 1 || out.Text != "aim check-in" {
		t.Fatalf("pushes=%d out=%+v", n, out)
	}
	if out.UserID != "" || out.ChatID != "" || out.SessionID != "" {
		t.Fatalf("push dest %+v", out)
	}
}

func TestRunner_PushUsesTextOnly(t *testing.T) {
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
	past := time.Now().UTC().Add(-time.Minute)
	if _, err := store.Schedule(ctx, "telegram leftover", cron.Parsed{
		Kind:     cron.KindOnce,
		Expr:     past.Format(time.RFC3339Nano),
		NextRun:  past,
		Timezone: "UTC",
	}, cron.Delivery{SessionID: "telegram:1:2", UserID: "2", ChatID: "1"}); err != nil {
		t.Fatal(err)
	}
	pusher := &memPusher{}
	handled := false
	runner := &cron.Runner{
		Store: store,
		Handle: func(_ context.Context, msg channel.Message) (string, error) {
			handled = true
			if msg.UserID != "" || msg.ChatID != "" {
				t.Fatalf("handle dest user=%q chat=%q", msg.UserID, msg.ChatID)
			}
			if msg.SessionID != "telegram:1:2" {
				t.Fatalf("session=%q", msg.SessionID)
			}
			return "hello", nil
		},
		Pusher: pusher,
	}
	runner.FireDueForTest(ctx)
	if !handled {
		t.Fatal("expected handle")
	}
	pusher.mu.Lock()
	defer pusher.mu.Unlock()
	if len(pusher.msgs) != 1 {
		t.Fatalf("pushes=%d", len(pusher.msgs))
	}
	out := pusher.msgs[0]
	if out.UserID != "" || out.ChatID != "" || out.SessionID != "" {
		t.Fatalf("push dest %+v", out)
	}
	if out.Text != "hello" || !strings.HasPrefix(out.ID, "cron-") {
		t.Fatalf("push %+v", out)
	}
}
