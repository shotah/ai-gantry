package telegram

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot"
)

func TestClipRunes(t *testing.T) {
	s := "hello世界"
	got := clipRunes(s, 5)
	if utf8.RuneCountInString(got) != 5 {
		t.Fatalf("got=%q count=%d", got, utf8.RuneCountInString(got))
	}
	if clipRunes("abc", 10) != "abc" {
		t.Fatal("no clip")
	}
}

func TestEditStream_CachesAndFlushes(t *testing.T) {
	prevFlush := streamFlushEvery
	streamFlushEvery = 30 * time.Millisecond
	t.Cleanup(func() { streamFlushEvery = prevFlush })

	var (
		mu    sync.Mutex
		edits int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		_, _ = io.ReadAll(r.Body)
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			mu.Lock()
			edits++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"text":"x"}}`))
		default:
			t.Errorf("unexpected method %q", method)
		}
	}))
	t.Cleanup(srv.Close)

	b, err := bot.New(testBotToken, bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	stream := newEditStream(b, 1, 0, 100)
	ctx := context.Background()
	if err := stream.Update(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	waitMsgID(t, stream)
	// Further tokens must be flushed via edit (not only the initial send).
	if err := stream.Update(ctx, "hello world"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		n := edits
		mu.Unlock()
		if n >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("edits=%d, want live flush", n)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := stream.Finish(ctx, "hello world final"); err != nil {
		t.Fatal(err)
	}
	stream.mu.Lock()
	flushed := stream.lastFlushed
	stream.mu.Unlock()
	if flushed != "hello world final" {
		t.Fatalf("lastFlushed=%q", flushed)
	}
}

func TestEditStream_CooldownOn429DoesNotBlockUpdate(t *testing.T) {
	prevFlush, prevBase := streamFlushEvery, retryBase
	streamFlushEvery = 20 * time.Millisecond
	retryBase = time.Millisecond
	t.Cleanup(func() {
		streamFlushEvery, retryBase = prevFlush, prevBase
	})

	var (
		mu    sync.Mutex
		edits int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		_, _ = io.ReadAll(r.Body)
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			mu.Lock()
			edits++
			mu.Unlock()
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":2}}`))
		default:
			t.Errorf("unexpected method %q", method)
		}
	}))
	t.Cleanup(srv.Close)

	b, err := bot.New(testBotToken, bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	stream := newEditStream(b, 1, 0, 100)
	ctx := context.Background()
	start := time.Now()
	if err := stream.Update(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	// Many updates must return immediately (cache only) even while flusher 429s.
	for i := 0; i < 20; i++ {
		if err := stream.Update(ctx, "hello "+strings.Repeat("x", i)); err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("Updates blocked for %v", time.Since(start))
	}
	waitMsgID(t, stream)
	if err := stream.Update(ctx, "hello after bubble"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		n := edits
		mu.Unlock()
		if n >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("expected at least one edit attempt after cooldown trigger")
		case <-time.After(10 * time.Millisecond):
		}
	}
	stream.stopFlusher()
}

// Thinking-only turn (model produced CoT but no answer): Finish must compose
// from the raw thinking/answer, never from the already-formatted display
// string — that double-renders the thinking with escaped HTML tags visible.
func TestEditStream_FinishThinkingOnlyNoDuplicate(t *testing.T) {
	prevFlush := streamFlushEvery
	streamFlushEvery = time.Hour
	t.Cleanup(func() { streamFlushEvery = prevFlush })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		_, _ = io.ReadAll(r.Body)
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"text":"x"}}`))
		default:
			t.Errorf("unexpected method %q", method)
		}
	}))
	t.Cleanup(srv.Close)

	b, err := bot.New(testBotToken, bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	stream := newEditStream(b, 1, 0, 4000)
	ctx := context.Background()
	if err := stream.UpdateThinking(ctx, "the user's plan is bounded", ""); err != nil {
		t.Fatal(err)
	}
	waitMsgID(t, stream)
	if err := stream.Finish(ctx, ""); err != nil {
		t.Fatal(err)
	}
	stream.mu.Lock()
	flushed := stream.lastFlushed
	stream.mu.Unlock()
	if n := strings.Count(flushed, "the user&#39;s plan is bounded"); n != 1 {
		t.Fatalf("thinking rendered %d times, want 1: %q", n, flushed)
	}
	if !strings.Contains(flushed, "<blockquote expandable>") {
		t.Fatalf("final not expandable: %q", flushed)
	}
	if strings.Contains(flushed, "&lt;i&gt;") || strings.Contains(flushed, "&amp;#39;") {
		t.Fatalf("double-escaped display leaked into final: %q", flushed)
	}
}

// Tool loop: iteration 2 streams a fresh CoT. Earlier reasoning must be
// archived and survive into the final collapsible, not erased mid-turn.
func TestEditStream_ThinkingAccumulatesAcrossIterations(t *testing.T) {
	prevFlush := streamFlushEvery
	streamFlushEvery = time.Hour
	t.Cleanup(func() { streamFlushEvery = prevFlush })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		_, _ = io.ReadAll(r.Body)
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"text":"x"}}`))
		default:
			t.Errorf("unexpected method %q", method)
		}
	}))
	t.Cleanup(srv.Close)

	b, err := bot.New(testBotToken, bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	stream := newEditStream(b, 1, 0, 4000)
	ctx := context.Background()
	// Iteration 1: CoT grows, then the model calls a tool.
	if err := stream.UpdateThinking(ctx, "plan the", ""); err != nil {
		t.Fatal(err)
	}
	if err := stream.UpdateThinking(ctx, "plan the calendar query", ""); err != nil {
		t.Fatal(err)
	}
	// Iteration 2: new model call, stream restarts from scratch.
	if err := stream.UpdateThinking(ctx, "summarize results", "Monday is busy"); err != nil {
		t.Fatal(err)
	}
	waitMsgID(t, stream)
	if err := stream.Finish(ctx, "Monday is busy"); err != nil {
		t.Fatal(err)
	}
	stream.mu.Lock()
	flushed := stream.lastFlushed
	stream.mu.Unlock()
	for _, want := range []string{"plan the calendar query", "summarize results", "Monday is busy"} {
		if !strings.Contains(flushed, want) {
			t.Fatalf("final missing %q: %q", want, flushed)
		}
	}
	// Growth within one call must not be archived as a separate copy.
	if n := strings.Count(flushed, "plan the"); n != 1 {
		t.Fatalf("iteration-1 thinking rendered %d times, want 1: %q", n, flushed)
	}
}

// After tools, the next model stream must not wipe earlier prose — traces sit
// inline between answer chunks (math → ✗ failed → explanation).
func TestEditStream_ToolLoopKeepsPriorAnswer(t *testing.T) {
	prevFlush := streamFlushEvery
	streamFlushEvery = time.Hour
	t.Cleanup(func() { streamFlushEvery = prevFlush })

	stream := newStubStream(t, 4000)
	ctx := context.Background()
	if err := stream.Update(ctx, "17.5% of 240 is 42."); err != nil {
		t.Fatal(err)
	}
	if err := stream.UpdateProgress(ctx, "→ strava__activities_get_zones"); err != nil {
		t.Fatal(err)
	}
	if err := stream.UpdateProgress(ctx, "✗ failed · 405ms"); err != nil {
		t.Fatal(err)
	}
	// Empty deltas after tools must not clear the math line.
	if err := stream.Update(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if got := streamLatest(stream); !strings.Contains(got, "17.5%") || !strings.Contains(got, "✗ failed") {
		t.Fatalf("mid-turn lost prose or trace: %q", got)
	}
	if err := stream.Update(ctx, "That activity id is missing from Strava."); err != nil {
		t.Fatal(err)
	}
	if err := stream.Finish(ctx, "That activity id is missing from Strava."); err != nil {
		t.Fatal(err)
	}
	stream.mu.Lock()
	flushed := stream.lastFlushed
	stream.mu.Unlock()
	for _, want := range []string{
		"17.5% of 240 is 42.",
		"strava__activities_get_zones",
		"✗ failed",
		"That activity id is missing from Strava.",
	} {
		if !strings.Contains(flushed, want) {
			t.Fatalf("final missing %q: %q", want, flushed)
		}
	}
	if i, j := strings.Index(flushed, "17.5%"), strings.Index(flushed, "strava__"); i < 0 || j < 0 || i > j {
		t.Fatalf("want math before tool trace: %q", flushed)
	}
	if i, j := strings.Index(flushed, "✗ failed"), strings.Index(flushed, "missing from Strava"); i < 0 || j < 0 || i > j {
		t.Fatalf("want failure trace before explanation: %q", flushed)
	}
}

// Tool trace lines must survive into the final collapsible and stay ordered
// relative to any CoT, so a long tool chain reads as progress.
func TestEditStream_ProgressTraceSurvivesFinish(t *testing.T) {
	prevFlush := streamFlushEvery
	streamFlushEvery = time.Hour
	t.Cleanup(func() { streamFlushEvery = prevFlush })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		_, _ = io.ReadAll(r.Body)
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"text":"x"}}`))
		default:
			t.Errorf("unexpected method %q", method)
		}
	}))
	t.Cleanup(srv.Close)

	b, err := bot.New(testBotToken, bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	stream := newEditStream(b, 1, 0, 4000)
	ctx := context.Background()
	if err := stream.UpdateThinking(ctx, "pick a tool", ""); err != nil {
		t.Fatal(err)
	}
	if err := stream.UpdateProgress(ctx, "→ garmin__activities_list"); err != nil {
		t.Fatal(err)
	}
	if err := stream.UpdateProgress(ctx, "✓ 1.2s · 4.1k chars"); err != nil {
		t.Fatal(err)
	}
	// Blank notes are no-ops (never blank out the bubble).
	if err := stream.UpdateProgress(ctx, "   "); err != nil {
		t.Fatal(err)
	}
	waitMsgID(t, stream)
	if err := stream.Finish(ctx, "You rode 21mi."); err != nil {
		t.Fatal(err)
	}
	stream.mu.Lock()
	flushed := stream.lastFlushed
	stream.mu.Unlock()
	for _, want := range []string{"pick a tool", "garmin__activities_list", "4.1k chars", "You rode 21mi."} {
		if !strings.Contains(flushed, want) {
			t.Fatalf("final missing %q: %q", want, flushed)
		}
	}
	if strings.Index(flushed, "pick a tool") > strings.Index(flushed, "garmin__activities_list") {
		t.Fatalf("trace ordered before thinking: %q", flushed)
	}
}

// newStubStream wires an editStream to a bot whose send/edit always succeed,
// for tests that only care about the composed text.
func newStubStream(t *testing.T, chunkMax int) *editStream {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		_, _ = io.ReadAll(r.Body)
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"text":"x"}}`))
		default:
			t.Errorf("unexpected method %q", method)
		}
	}))
	t.Cleanup(srv.Close)
	b, err := bot.New(testBotToken, bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	return newEditStream(b, 1, 0, chunkMax)
}

// The spin-up notice is a waiting indicator: it opens the bubble during silent
// prefill, coexists with tool traces, and is gone once the reply lands.
func TestEditStream_StatusLineIsReplacedByReply(t *testing.T) {
	prevFlush := streamFlushEvery
	streamFlushEvery = time.Hour
	t.Cleanup(func() { streamFlushEvery = prevFlush })

	stream := newStubStream(t, 4000)
	ctx := context.Background()
	if err := stream.UpdateStatus(ctx, "⏳ spinning up"); err != nil {
		t.Fatal(err)
	}
	// Nothing else has been said yet, so the notice alone opens the bubble.
	waitMsgID(t, stream)
	if got := streamLatest(stream); !strings.Contains(got, "spinning up") {
		t.Fatalf("bubble missing notice: %q", got)
	}
	// A tool trace and the notice describe different things; both belong.
	if err := stream.UpdateProgress(ctx, "→ garmin__activities_list"); err != nil {
		t.Fatal(err)
	}
	got := streamLatest(stream)
	if !strings.Contains(got, "garmin__activities_list") || !strings.Contains(got, "spinning up") {
		t.Fatalf("bubble = %q, want trace + notice", got)
	}

	if err := stream.UpdateStatus(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := stream.UpdateThinking(ctx, "", "You rode 21mi."); err != nil {
		t.Fatal(err)
	}
	if err := stream.Finish(ctx, "You rode 21mi."); err != nil {
		t.Fatal(err)
	}
	stream.mu.Lock()
	flushed := stream.lastFlushed
	stream.mu.Unlock()
	if strings.Contains(flushed, "spinning up") {
		t.Fatalf("notice survived into the reply: %q", flushed)
	}
	for _, want := range []string{"garmin__activities_list", "You rode 21mi."} {
		if !strings.Contains(flushed, want) {
			t.Fatalf("final missing %q: %q", want, flushed)
		}
	}
}

// Error and cancel paths jump straight to Finish, which must still drop it.
func TestEditStream_FinishDropsLingeringStatus(t *testing.T) {
	prevFlush := streamFlushEvery
	streamFlushEvery = time.Hour
	t.Cleanup(func() { streamFlushEvery = prevFlush })

	stream := newStubStream(t, 4000)
	ctx := context.Background()
	if err := stream.UpdateStatus(ctx, "⏳ spinning up"); err != nil {
		t.Fatal(err)
	}
	waitMsgID(t, stream)
	if err := stream.Finish(ctx, "model call failed"); err != nil {
		t.Fatal(err)
	}
	stream.mu.Lock()
	flushed := stream.lastFlushed
	stream.mu.Unlock()
	if flushed != "model call failed" {
		t.Fatalf("lastFlushed = %q, want the error text alone", flushed)
	}
}

func streamLatest(s *editStream) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

func TestEditStream_FinishNotModifiedIsOK(t *testing.T) {
	prevFlush := streamFlushEvery
	streamFlushEvery = time.Hour
	t.Cleanup(func() { streamFlushEvery = prevFlush })

	var edits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		_, _ = io.ReadAll(r.Body)
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			edits++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message"}`))
		default:
			t.Errorf("unexpected method %q", method)
		}
	}))
	t.Cleanup(srv.Close)

	b, err := bot.New(testBotToken, bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	stream := newEditStream(b, 1, 0, 100)
	ctx := context.Background()
	if err := stream.sendInitial(ctx, "same text"); err != nil {
		t.Fatal(err)
	}
	// Force lastFlushed to differ so Finish actually hits editMessageText.
	stream.mu.Lock()
	stream.lastFlushed = "stale"
	stream.mu.Unlock()
	if err := stream.Finish(ctx, "same text"); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if edits != 1 {
		t.Fatalf("edits=%d want 1", edits)
	}
}

func TestEditStream_FinishDeliversAfter429(t *testing.T) {
	prevFlush, prevBase, prevAttempts := streamFlushEvery, retryBase, retryAttempts
	streamFlushEvery = time.Hour // flusher won't help; Finish must deliver
	retryBase = time.Millisecond
	retryAttempts = 3
	t.Cleanup(func() {
		streamFlushEvery, retryBase, retryAttempts = prevFlush, prevBase, prevAttempts
	})

	var (
		mu    sync.Mutex
		edits int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		_, _ = io.ReadAll(r.Body)
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			mu.Lock()
			edits++
			n := edits
			mu.Unlock()
			if n < 3 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"retry","parameters":{"retry_after":0}}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"text":"done"}}`))
		default:
			t.Errorf("unexpected method %q", method)
		}
	}))
	t.Cleanup(srv.Close)

	b, err := bot.New(testBotToken, bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	stream := newEditStream(b, 1, 0, 100)
	ctx := context.Background()
	// Seed message id without relying on flusher.
	if err := stream.sendInitial(ctx, "partial"); err != nil {
		t.Fatal(err)
	}
	if err := stream.Update(ctx, "partial cached"); err != nil {
		t.Fatal(err)
	}
	if err := stream.Finish(ctx, "full final reply the user must see"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if edits != 3 {
		t.Fatalf("edits=%d want 3", edits)
	}
}
