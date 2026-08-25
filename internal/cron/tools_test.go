package cron_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestToolDefsAndIsCronTool(t *testing.T) {
	defs := cron.ToolDefs()
	if len(defs) != 3 {
		t.Fatalf("defs=%d", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, n := range []string{cron.ToolSchedule, cron.ToolList, cron.ToolCancel} {
		if !names[n] || !cron.IsCronTool(n) {
			t.Fatalf("missing %s", n)
		}
	}
	if cron.IsCronTool("nope") {
		t.Fatal("expected false")
	}
}

func TestTools_CancelAndList(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if store.MaxJobs() != 5 {
		t.Fatalf("MaxJobs=%d", store.MaxJobs())
	}

	tools := cron.Tools{Store: store, TZ: "UTC"}
	ctx = cron.WithDelivery(ctx, cron.Delivery{SessionID: "stdio", UserID: "local", ChatID: "1"})
	out, err := tools.Call(ctx, cron.ToolSchedule, json.RawMessage(`{"prompt":"ping","when":"in 1h"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty schedule result")
	}
	list, err := tools.Call(ctx, cron.ToolList, json.RawMessage(`{}`))
	if err != nil || list == "no active cron jobs" {
		t.Fatalf("list=%q err=%v", list, err)
	}
	jobs, err := store.List(ctx, false)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	// string id exercises asInt64 string branch
	cancelArgs := []byte(`{"id":"` + strconv.FormatInt(jobs[0].ID, 10) + `"}`)
	if _, err := tools.Call(ctx, cron.ToolCancel, cancelArgs); err != nil {
		t.Fatal(err)
	}
	if err := store.Cancel(ctx, 99999); err == nil {
		t.Fatal("expected missing cancel error")
	}
}

func TestTools_ScheduleWithMemoryPin(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 5)
	if err != nil {
		t.Fatal(err)
	}
	mem, err := memory.OpenDB(sess.DB())
	if err != nil {
		t.Fatal(err)
	}
	row, err := mem.Store(ctx, memory.KindFact, "follow/passport", "renew next month")
	if err != nil {
		t.Fatal(err)
	}
	tools := cron.Tools{Store: store, TZ: "UTC", Memory: mem}
	ctx = cron.WithDelivery(ctx, cron.Delivery{SessionID: "stdio", UserID: "local", ChatID: "1"})
	args := []byte(`{"prompt":"check passport","when":"in 1h","memory_id":` + strconv.FormatInt(row.ID, 10) + `}`)
	out, err := tools.Call(ctx, cron.ToolSchedule, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "memory_id="+strconv.FormatInt(row.ID, 10)) || !strings.Contains(out, "follow/passport") {
		t.Fatalf("schedule pin: %q", out)
	}
	list, err := tools.Call(ctx, cron.ToolList, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, "follow/passport") {
		t.Fatalf("list pin: %q", list)
	}
	if _, err := tools.Call(ctx, cron.ToolSchedule, json.RawMessage(`{"prompt":"x","when":"in 1h","memory_id":99999}`)); err == nil {
		t.Fatal("expected missing memory_id error")
	}
}

func TestComposite_Routes(t *testing.T) {
	sess, err := session.Open(t.TempDir(), 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}
	other := &stubTools{defs: []provider.ToolDef{{Name: "x__y"}}}
	c := cron.Composite{
		Cron:  cron.Tools{Store: store, TZ: "UTC"},
		Other: other,
	}
	if c.ToolCount() < 4 {
		t.Fatalf("count=%d", c.ToolCount())
	}
	ctx := cron.WithDelivery(context.Background(), cron.Delivery{SessionID: "s", UserID: "u"})
	if _, err := c.Call(ctx, cron.ToolList, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call(ctx, "x__y", nil); err != nil {
		t.Fatal(err)
	}
	if other.calls != 1 {
		t.Fatalf("other calls=%d", other.calls)
	}
}

type stubTools struct {
	defs  []provider.ToolDef
	calls int
}

func (s *stubTools) Tools() []provider.ToolDef { return s.defs }

func (s *stubTools) ToolCount() int { return len(s.defs) }

func (s *stubTools) Call(context.Context, string, json.RawMessage) (string, error) {
	s.calls++
	return "ok", nil
}
