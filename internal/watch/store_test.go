package watch_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
	"github.com/shotah/ai-gantry/internal/watch"
)

type watchFixture struct {
	store *watch.Store
	db    *sql.DB
}

func openWatchFixture(t *testing.T, maxWatches int) watchFixture {
	t.Helper()
	if maxWatches < 1 {
		maxWatches = 50
	}
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := watch.OpenDB(sess.DB(), maxWatches)
	if err != nil {
		t.Fatal(err)
	}
	return watchFixture{store: store, db: sess.DB()}
}

func TestStore_AddDueClaimFinishCancel(t *testing.T) {
	ctx := context.Background()
	f := openWatchFixture(t, 5)
	if f.store.MaxWatches() != 5 {
		t.Fatalf("MaxWatches=%d", f.store.MaxWatches())
	}
	delivery := cron.Delivery{SessionID: "s1", UserID: "u", ChatID: "c"}
	w, err := f.store.Add(ctx, "feeds__items_list", []byte(`{"url":"https://x"}`), "nws", 15*time.Minute, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if w.ID == 0 || w.Tool != "feeds__items_list" || w.Label != "nws" {
		t.Fatalf("watch=%+v", w)
	}
	due, err := f.store.Due(ctx, time.Now().UTC().Add(time.Second), 10)
	if err != nil || len(due) != 1 || due[0].ID != w.ID {
		t.Fatalf("due=%v err=%v", due, err)
	}
	ok, err := f.store.Claim(ctx, w.ID, time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	ok, err = f.store.Claim(ctx, w.ID, time.Now().UTC())
	if err != nil || ok {
		t.Fatalf("second claim should miss, ok=%v err=%v", ok, err)
	}
	if err := f.store.Finish(ctx, w, []string{"a", "b"}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Running || len(got.SeenIDs) != 2 || got.SeenIDs[0] != "a" {
		t.Fatalf("after finish: %+v", got)
	}
	if !got.NextRunAt.After(time.Now().UTC()) {
		t.Fatalf("next_run should be in the future: %v", got.NextRunAt)
	}
	if err := f.store.Cancel(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	got, err = f.store.Get(ctx, w.ID)
	if err != nil || got.Enabled {
		t.Fatalf("cancelled enabled=%v err=%v", got.Enabled, err)
	}
	if err := f.store.Cancel(ctx, 999); err == nil {
		t.Fatal("expected missing cancel")
	}
}

func TestStore_RejectsBadToolAndMax(t *testing.T) {
	ctx := context.Background()
	f := openWatchFixture(t, 1)
	d := cron.Delivery{SessionID: "s"}
	if _, err := f.store.Add(ctx, "memory_store", nil, "", time.Minute, d); err == nil {
		t.Fatal("expected unprefixed tool error")
	}
	if _, err := f.store.Add(ctx, "feeds__items_list", nil, "", time.Minute, cron.Delivery{}); err == nil {
		t.Fatal("expected missing session")
	}
	if _, err := f.store.Add(ctx, "feeds__items_list", nil, "", time.Minute, d); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Add(ctx, "feeds__other", nil, "", time.Minute, d); err == nil {
		t.Fatal("expected max watches")
	}
}

func TestClearStaleRunning_RecoversDueWatches(t *testing.T) {
	ctx := context.Background()
	f := openWatchFixture(t, 5)
	w, err := f.store.Add(ctx, "feeds__items_list", nil, "", time.Minute, cron.Delivery{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE watch SET running = 1 WHERE id = ?`, w.ID); err != nil {
		t.Fatal(err)
	}
	due, err := f.store.Due(ctx, time.Now().UTC(), 10)
	if err != nil || len(due) != 0 {
		t.Fatalf("stuck running should be invisible, due=%v err=%v", due, err)
	}
	n, err := f.store.ClearStaleRunning(ctx)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	due, err = f.store.Due(ctx, time.Now().UTC(), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("after clear due=%v err=%v", due, err)
	}
}

func TestFinish_DoesNotResurrectCancelledWatch(t *testing.T) {
	ctx := context.Background()
	f := openWatchFixture(t, 5)
	w, err := f.store.Add(ctx, "feeds__items_list", nil, "", time.Minute, cron.Delivery{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := f.store.Claim(ctx, w.ID, time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if err := f.store.Cancel(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Finish(ctx, w, []string{"x"}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Get(ctx, w.ID)
	if err != nil || got.Enabled {
		t.Fatalf("should stay cancelled: %+v err=%v", got, err)
	}
}

func TestOpenDB_Nil(t *testing.T) {
	if _, err := watch.OpenDB(nil, 0); err == nil {
		t.Fatal("expected nil db error")
	}
}

func TestStore_Edges(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := watch.OpenDB(sess.DB(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if store.MaxWatches() != 50 {
		t.Fatalf("default max=%d", store.MaxWatches())
	}
	f := watchFixture{store: store, db: sess.DB()}
	if _, err := f.store.Add(ctx, "feeds__items_list", []byte("null"), "", time.Second, cron.Delivery{SessionID: "s"}); err == nil {
		t.Fatal("expected short interval")
	}
	w, err := f.store.Add(ctx, "feeds__items_list", []byte("{"), "x", time.Minute, cron.Delivery{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if string(w.Args) != "{}" {
		t.Fatalf("bad json args should normalize, got %s", w.Args)
	}
	if _, err := f.store.Get(ctx, 999); err == nil {
		t.Fatal("expected missing get")
	}
	if err := f.store.ForceDueForTest(ctx, 999); err == nil {
		t.Fatal("expected missing force-due")
	}
	if err := f.store.Cancel(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	list, err := f.store.ListSession(ctx, "s", true)
	if err != nil || len(list) != 1 || list[0].Enabled {
		t.Fatalf("includeDisabled=%v err=%v", list, err)
	}
	due, err := f.store.Due(ctx, time.Now().UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("cancelled should not be due: %v", due)
	}
	w2, err := f.store.Add(ctx, "feeds__other", nil, "", time.Minute, cron.Delivery{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	due, err = f.store.Due(ctx, time.Now().UTC(), 0)
	if err != nil || len(due) != 1 || due[0].ID != w2.ID {
		t.Fatalf("limit<1 should still return due: %v err=%v", due, err)
	}
}
