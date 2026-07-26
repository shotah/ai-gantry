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
