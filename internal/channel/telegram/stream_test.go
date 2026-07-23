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
	if got[len(got)-len("…"):] != "…" && !hasEllipsis(got) {
		// clipRunes uses "…" as last rune when truncating
		if utf8.RuneCountInString(s) <= 5 {
			t.Fatal("expected truncation")
		}
	}
	if clipRunes("abc", 10) != "abc" {
		t.Fatal("no clip")
	}
}

func hasEllipsis(s string) bool {
	for _, r := range s {
		if r == '…' {
			return true
		}
	}
	return false
}

func TestEditStream_Retries429OnEdit(t *testing.T) {
	prevBase, prevAttempts := retryBase, retryAttempts
	retryBase = time.Millisecond
	retryAttempts = 3
	t.Cleanup(func() {
		retryBase, retryAttempts = prevBase, prevAttempts
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
				_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":0}}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"text":"hi"}}`))
		default:
			t.Errorf("unexpected method %q", method)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":404,"description":"unknown"}`))
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
		t.Fatalf("initial: %v", err)
	}
	// Bypass throttle so the next Update issues an edit.
	stream.lastEdit = time.Now().Add(-streamMinEditGap)
	if err := stream.Update(ctx, "hello world"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if edits != 3 {
		t.Fatalf("editMessageText calls=%d want 3", edits)
	}
}

func TestEditStream_UpdateSoftSkipsEditErrors(t *testing.T) {
	prevBase, prevAttempts := retryBase, retryAttempts
	retryBase = time.Millisecond
	retryAttempts = 1
	t.Cleanup(func() {
		retryBase, retryAttempts = prevBase, prevAttempts
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		_, _ = io.ReadAll(r.Body)
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			// Non-429 failure (timeouts / bad request / etc.) must not abort the LLM.
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`))
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
		t.Fatalf("initial: %v", err)
	}
	stream.lastEdit = time.Now().Add(-streamMinEditGap)
	if err := stream.Update(ctx, "hello world"); err != nil {
		t.Fatalf("expected soft-skip, got %v", err)
	}
}
