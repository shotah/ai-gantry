package watch_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
	"github.com/shotah/ai-gantry/internal/watch"
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

type scriptFetcher struct {
	mu      sync.Mutex
	results []string
	errs    []error
	calls   int
}

func (f *scriptFetcher) Call(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return "", f.errs[i]
	}
	if i < len(f.results) {
		return f.results[i], nil
	}
	if len(f.results) > 0 {
		return f.results[len(f.results)-1], nil
	}
	return `{"items":[]}`, nil
}

func newWatchRunner(t *testing.T, fetch *scriptFetcher, handle channel.Handler, pusher *memPusher) (*watch.Runner, *watch.Store) {
	t.Helper()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := watch.OpenDB(sess.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}
	return &watch.Runner{
		Store:   store,
		Fetcher: fetch,
		Handle:  handle,
		Pusher:  pusher,
		Logger:  slog.Default(),
	}, store
}

func TestRunner_FirstPollSeedsWithoutHandle(t *testing.T) {
	ctx := context.Background()
	fetch := &scriptFetcher{results: []string{`{"items":[{"id":"old1"},{"id":"old2"}]}`}}
	var handled int
	pusher := &memPusher{}
	runner, store := newWatchRunner(t, fetch, func(context.Context, channel.Message) (string, error) {
		handled++
		return "should not run", nil
	}, pusher)
	_, err := store.Add(ctx, "feeds__items_list", []byte(`{"url":"https://x"}`), "blog", time.Minute, cron.Delivery{
		SessionID: "s", UserID: "u", ChatID: "c",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner.FireDueForTest(ctx)
	if handled != 0 {
		t.Fatalf("handle calls=%d", handled)
	}
	pusher.mu.Lock()
	n := len(pusher.msgs)
	pusher.mu.Unlock()
	if n != 0 {
		t.Fatalf("pushes=%d", n)
	}
	list, err := store.ListSession(ctx, "s", false)
	if err != nil || len(list) != 1 || len(list[0].SeenIDs) != 2 {
		t.Fatalf("seeded=%v err=%v", list, err)
	}
}

func TestRunner_NewItemWakesAndPushes(t *testing.T) {
	ctx := context.Background()
	fetch := &scriptFetcher{results: []string{`{"items":[{"id":"a"},{"id":"b","title":"fresh"}]}`}}
	var handled string
	pusher := &memPusher{}
	runner, store := newWatchRunner(t, fetch, func(_ context.Context, msg channel.Message) (string, error) {
		handled = msg.Text
		return "b posted", nil
	}, pusher)
	w, err := store.Add(ctx, "feeds__items_list", []byte(`{}`), "blog", time.Minute, cron.Delivery{
		SessionID: "s", UserID: "u", ChatID: "c",
	})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := store.Claim(ctx, w.ID, time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if err := store.Finish(ctx, w, []string{"a"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.ForceDueForTest(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	runner.FireDueForTest(ctx)
	if handled == "" || !strings.HasPrefix(handled, "[watch]") {
		t.Fatalf("handle text=%q", handled)
	}
	if !strings.Contains(handled, "id=b") || !strings.Contains(handled, "fresh") || !strings.Contains(handled, "do not re-fetch") {
		t.Fatalf("handle missing items: %q", handled)
	}
	if strings.Contains(handled, "id=a title") {
		t.Fatalf("old item should not be in wake: %q", handled)
	}
	pusher.mu.Lock()
	n := len(pusher.msgs)
	pusher.mu.Unlock()
	if n != 1 {
		t.Fatalf("pushes=%d", n)
	}
}

func TestRunner_SilentReplySkipsPush(t *testing.T) {
	ctx := context.Background()
	fetch := &scriptFetcher{results: []string{`{"items":[{"id":"n"}]}`}}
	pusher := &memPusher{}
	runner, store := newWatchRunner(t, fetch, func(context.Context, channel.Message) (string, error) {
		return "[silent]\nnoise", nil
	}, pusher)
	w, err := store.Add(ctx, "feeds__items_list", []byte(`{}`), "", time.Minute, cron.Delivery{SessionID: "s", UserID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := store.Claim(ctx, w.ID, time.Now().UTC())
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, w, []string{"old"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.ForceDueForTest(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	runner.FireDueForTest(ctx)
	pusher.mu.Lock()
	n := len(pusher.msgs)
	pusher.mu.Unlock()
	if n != 0 {
		t.Fatalf("silent should not push, got %d", n)
	}
}

func TestRunner_FetchErrorDoesNotHandle(t *testing.T) {
	ctx := context.Background()
	fetch := &scriptFetcher{errs: []error{context.DeadlineExceeded}}
	var handled int
	runner, store := newWatchRunner(t, fetch, func(context.Context, channel.Message) (string, error) {
		handled++
		return "x", nil
	}, &memPusher{})
	w, err := store.Add(ctx, "feeds__items_list", []byte(`{}`), "", time.Minute, cron.Delivery{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := store.Claim(ctx, w.ID, time.Now().UTC())
	if !ok {
		t.Fatal("claim")
	}
	_ = store.Finish(ctx, w, []string{"seed"}, nil)
	if err := store.ForceDueForTest(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	runner.FireDueForTest(ctx)
	if handled != 0 {
		t.Fatalf("handled=%d", handled)
	}
	got, err := store.Get(ctx, w.ID)
	if err != nil || got.LastError == "" {
		t.Fatalf("expected last_error, got %+v err=%v", got, err)
	}
}

func TestRunner_QuietTickNoHandle(t *testing.T) {
	ctx := context.Background()
	fetch := &scriptFetcher{results: []string{`{"items":[{"id":"a"}]}`}}
	var handled int
	runner, store := newWatchRunner(t, fetch, func(context.Context, channel.Message) (string, error) {
		handled++
		return "x", nil
	}, &memPusher{})
	w, err := store.Add(ctx, "feeds__items_list", []byte(`{}`), "", time.Minute, cron.Delivery{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := store.Claim(ctx, w.ID, time.Now().UTC())
	if !ok {
		t.Fatal("claim")
	}
	_ = store.Finish(ctx, w, []string{"a"}, nil)
	if err := store.ForceDueForTest(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	runner.FireDueForTest(ctx)
	if handled != 0 {
		t.Fatalf("quiet tick handled=%d", handled)
	}
}

type failPusher struct{}

func (failPusher) Push(context.Context, channel.Outbound) error { return context.Canceled }

func TestRunner_ParseAndHandleErrors(t *testing.T) {
	ctx := context.Background()
	fetch := &scriptFetcher{results: []string{`not-json`}}
	var handled int
	runner, store := newWatchRunner(t, fetch, func(context.Context, channel.Message) (string, error) {
		handled++
		return "x", nil
	}, &memPusher{})
	w, err := store.Add(ctx, "feeds__items_list", []byte(`{}`), "", time.Minute, cron.Delivery{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := store.Claim(ctx, w.ID, time.Now().UTC())
	if !ok {
		t.Fatal("claim")
	}
	_ = store.Finish(ctx, w, []string{"seed"}, nil)
	if err := store.ForceDueForTest(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	runner.FireDueForTest(ctx)
	if handled != 0 {
		t.Fatalf("parse error handled=%d", handled)
	}
	got, err := store.Get(ctx, w.ID)
	if err != nil || got.LastError == "" {
		t.Fatalf("expected parse last_error %+v err=%v", got, err)
	}
}

func TestRunner_HandleErrorAndEmptyReply(t *testing.T) {
	ctx := context.Background()
	fetch := &scriptFetcher{results: []string{`{"items":[{"id":"n"}]}`}}
	pusher := &memPusher{}
	runner, store := newWatchRunner(t, fetch, func(context.Context, channel.Message) (string, error) {
		return "", context.Canceled
	}, pusher)
	w, err := store.Add(ctx, "feeds__items_list", []byte(`{}`), "lab", time.Minute, cron.Delivery{SessionID: "s", UserID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := store.Claim(ctx, w.ID, time.Now().UTC())
	if !ok {
		t.Fatal("claim")
	}
	_ = store.Finish(ctx, w, []string{"old"}, nil)
	if err := store.ForceDueForTest(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	runner.FireDueForTest(ctx)
	pusher.mu.Lock()
	n := len(pusher.msgs)
	pusher.mu.Unlock()
	if n != 0 {
		t.Fatalf("handle error should not push, got %d", n)
	}

	empty := &memPusher{}
	runner2, store2 := newWatchRunner(t, &scriptFetcher{results: []string{`{"items":[{"id":"n"}]}`}},
		func(context.Context, channel.Message) (string, error) { return "", nil }, empty)
	w2, err := store2.Add(ctx, "feeds__items_list", []byte(`{}`), "", time.Minute, cron.Delivery{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ok, _ = store2.Claim(ctx, w2.ID, time.Now().UTC())
	if !ok {
		t.Fatal("claim")
	}
	_ = store2.Finish(ctx, w2, []string{"old"}, nil)
	_ = store2.ForceDueForTest(ctx, w2.ID)
	runner2.FireDueForTest(ctx)
	empty.mu.Lock()
	n = len(empty.msgs)
	empty.mu.Unlock()
	if n != 0 {
		t.Fatalf("empty reply should not push, got %d", n)
	}
}

func TestRunner_PushErrorAndStart(t *testing.T) {
	ctx := context.Background()
	fetch := &scriptFetcher{results: []string{`{"items":[{"id":"n"}]}`}}
	runner, store := newWatchRunner(t, fetch, func(context.Context, channel.Message) (string, error) {
		return "hello", nil
	}, &memPusher{})
	runner.Pusher = failPusher{}
	w, err := store.Add(ctx, "feeds__items_list", []byte(`{}`), "", time.Minute, cron.Delivery{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := store.Claim(ctx, w.ID, time.Now().UTC())
	if !ok {
		t.Fatal("claim")
	}
	_ = store.Finish(ctx, w, []string{"old"}, nil)
	_ = store.ForceDueForTest(ctx, w.ID)
	runner.FireDueForTest(ctx)
	got, err := store.Get(ctx, w.ID)
	if err != nil || got.LastError == "" {
		t.Fatalf("expected push last_error %+v err=%v", got, err)
	}

	(&watch.Runner{}).Start(context.Background())
	seedFetch := &scriptFetcher{results: []string{`{"items":[{"id":"a"}]}`}}
	r2, s2 := newWatchRunner(t, seedFetch, func(context.Context, channel.Message) (string, error) {
		return "no", nil
	}, &memPusher{})
	if _, err := s2.Add(ctx, "feeds__items_list", []byte(`{}`), "", time.Minute, cron.Delivery{SessionID: "s"}); err != nil {
		t.Fatal(err)
	}
	done, cancel := context.WithCancel(context.Background())
	cancel()
	r2.Logger = nil
	r2.Start(done)
}
