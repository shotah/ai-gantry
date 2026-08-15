package watch_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
	"github.com/shotah/ai-gantry/internal/watch"
)

func TestToolDefsAndIsWatchTool(t *testing.T) {
	defs := watch.ToolDefs()
	if len(defs) != 3 {
		t.Fatalf("defs=%d", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, n := range []string{watch.ToolAdd, watch.ToolList, watch.ToolCancel} {
		if !names[n] || !watch.IsWatchTool(n) {
			t.Fatalf("missing %s", n)
		}
	}
	if watch.IsWatchTool("nope") {
		t.Fatal("expected false")
	}
}

func TestTools_AddListCancel(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := watch.OpenDB(sess.DB(), 5)
	if err != nil {
		t.Fatal(err)
	}
	tools := watch.Tools{Store: store}
	ctx = cron.WithDelivery(ctx, cron.Delivery{SessionID: "stdio", UserID: "local", ChatID: "1"})
	out, err := tools.Call(ctx, watch.ToolAdd, json.RawMessage(
		`{"tool":"feeds__items_list","args":{"url":"https://x"},"interval":"15m","label":"blog"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty add result")
	}
	list, err := tools.Call(ctx, watch.ToolList, json.RawMessage(`{}`))
	if err != nil || list == "no active watches" {
		t.Fatalf("list=%q err=%v", list, err)
	}
	watches, err := store.ListSession(ctx, "stdio", false)
	if err != nil || len(watches) != 1 {
		t.Fatalf("watches=%v err=%v", watches, err)
	}
	cancelArgs := []byte(`{"id":"` + strconv.FormatInt(watches[0].ID, 10) + `"}`)
	if _, err := tools.Call(ctx, watch.ToolCancel, cancelArgs); err != nil {
		t.Fatal(err)
	}
	list, err = tools.Call(ctx, watch.ToolList, json.RawMessage(`{}`))
	if err != nil || list != "no active watches" {
		t.Fatalf("after cancel list=%q err=%v", list, err)
	}
}

func TestTools_RequiresDeliveryAndPrefixedTool(t *testing.T) {
	ctx := context.Background()
	f := openWatchFixture(t, 5)
	tools := watch.Tools{Store: f.store}
	if _, err := tools.Call(ctx, watch.ToolAdd, json.RawMessage(`{"tool":"feeds__items_list"}`)); err == nil {
		t.Fatal("expected missing delivery")
	}
	ctx = cron.WithDelivery(ctx, cron.Delivery{SessionID: "s"})
	if _, err := tools.Call(ctx, watch.ToolAdd, json.RawMessage(`{"tool":"memory_store"}`)); err == nil {
		t.Fatal("expected unprefixed tool")
	}
	if _, err := tools.Call(ctx, "nope", nil); err == nil {
		t.Fatal("expected unknown tool")
	}
	if _, err := tools.Call(ctx, watch.ToolAdd, json.RawMessage(`{`)); err == nil {
		t.Fatal("expected bad arguments")
	}
}

func TestTools_NilStoreAndSessionGuard(t *testing.T) {
	ctx := cron.WithDelivery(context.Background(), cron.Delivery{SessionID: "s"})
	if _, err := (watch.Tools{}).Call(ctx, watch.ToolList, nil); err == nil {
		t.Fatal("expected nil store")
	}
	f := openWatchFixture(t, 5)
	tools := watch.Tools{Store: f.store}
	out, err := tools.Call(ctx, watch.ToolAdd, json.RawMessage(`{"tool":"feeds__items_list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty add")
	}
	other := cron.WithDelivery(context.Background(), cron.Delivery{SessionID: "other"})
	if _, err := tools.Call(other, watch.ToolCancel, json.RawMessage(`{"id":1}`)); err == nil {
		t.Fatal("expected other-session cancel reject")
	}
	if _, err := tools.Call(ctx, watch.ToolCancel, json.RawMessage(`{"id":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.Call(ctx, watch.ToolCancel, json.RawMessage(`{"id":true}`)); err == nil {
		t.Fatal("expected invalid id")
	}
}
