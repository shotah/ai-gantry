package watch_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/watch"
)

// A watch on a budgeted server cannot be born faster than the budget: the
// 15m default (or a "1h" the model typed) is raised to the floor and the
// reply says so, so the model does not promise hourly news.
func TestTools_AddRaisesIntervalToFloor(t *testing.T) {
	ctx := cron.WithDelivery(context.Background(), cron.Delivery{SessionID: "s", UserID: "u", ChatID: "c"})
	f := openWatchFixture(t, 5)
	tools := watch.Tools{
		Store: f.store,
		Floor: func(tool string) time.Duration {
			if strings.HasPrefix(tool, "rentals__") {
				return 24 * time.Hour
			}
			return 0
		},
	}
	out, err := tools.Call(ctx, watch.ToolAdd, json.RawMessage(
		`{"tool":"rentals__listings_search","args":{"city":"Denver"},"label":"denver rentals"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "interval=24h0m0s") || !strings.Contains(out, "interval raised to 24h0m0s") {
		t.Fatalf("out=%q", out)
	}
	ws, err := f.store.ListSession(ctx, "s", false)
	if err != nil || len(ws) != 1 || ws[0].IntervalSeconds != int(24*time.Hour/time.Second) {
		t.Fatalf("watches=%+v err=%v", ws, err)
	}

	// No floor for this server: the request stands.
	out, err = tools.Call(ctx, watch.ToolAdd, json.RawMessage(`{"tool":"feeds__items_list","interval":"15m"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "interval=15m0s") || strings.Contains(out, "raised") {
		t.Fatalf("out=%q", out)
	}
}

type budgetErr struct{ at time.Time }

func (e *budgetErr) Error() string { return "mcp: rentals budget 1/day used" }

func (e *budgetErr) RetryAfter() time.Time { return e.at }

// A refused poll parks the watch until the budget resets — not the same
// 15m interval, which would log 95 refusals a day into the same wall.
// The watch stays enabled and keeps its cursor.
func TestRunner_DefersRefusedPollUntilRetryAfter(t *testing.T) {
	ctx := context.Background()
	reset := time.Now().Add(9 * time.Hour)
	fetch := &scriptFetcher{errs: []error{&budgetErr{at: reset}}}
	pusher := &memPusher{}
	runner, store := newWatchRunner(t, fetch, func(context.Context, channel.Message) (string, error) {
		t.Fatal("handle must not run on a refused poll")
		return "", nil
	}, pusher)
	w, err := store.Add(ctx, "rentals__listings_search", []byte(`{}`), "denver", 15*time.Minute, cron.Delivery{
		SessionID: "s", UserID: "u", ChatID: "c",
	})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := store.Claim(ctx, w.ID, time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if err := store.Finish(ctx, w, []string{"seen1"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.ForceDueForTest(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	runner.FireDueForTest(ctx)

	ws, err := store.ListSession(ctx, "s", false)
	if err != nil || len(ws) != 1 {
		t.Fatalf("watches=%+v err=%v", ws, err)
	}
	got := ws[0]
	if !got.Enabled {
		t.Fatal("watch was disabled by a budget refusal")
	}
	if len(got.SeenIDs) != 1 || got.SeenIDs[0] != "seen1" {
		t.Fatalf("cursor lost: %v", got.SeenIDs)
	}
	if d := got.NextRunAt.Sub(reset); d < -2*time.Second || d > 2*time.Second {
		t.Fatalf("next_run_at=%v want ≈ %v (reset), got delta %s", got.NextRunAt, reset, d)
	}
	if !strings.Contains(got.LastError, "budget") {
		t.Fatalf("last_error=%q", got.LastError)
	}
	if fetch.calls != 1 {
		t.Fatalf("fetch calls=%d", fetch.calls)
	}
}

// A plain fetch error still reschedules at the interval — deferral is only
// for errors that say when to come back.
func TestRunner_PlainErrorKeepsInterval(t *testing.T) {
	ctx := context.Background()
	fetch := &scriptFetcher{errs: []error{errors.New("boom")}}
	runner, store := newWatchRunner(t, fetch, func(context.Context, channel.Message) (string, error) {
		return "", nil
	}, &memPusher{})
	w, err := store.Add(ctx, "feeds__items_list", []byte(`{}`), "", 15*time.Minute, cron.Delivery{
		SessionID: "s", UserID: "u", ChatID: "c",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ForceDueForTest(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	runner.FireDueForTest(ctx)
	ws, err := store.ListSession(ctx, "s", false)
	if err != nil || len(ws) != 1 {
		t.Fatalf("watches=%+v err=%v", ws, err)
	}
	if d := ws[0].NextRunAt.Sub(before.Add(15 * time.Minute)); d < -2*time.Second || d > 2*time.Second {
		t.Fatalf("next_run_at=%v, want ≈ now+15m", ws[0].NextRunAt)
	}
}
