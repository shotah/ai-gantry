package logfwd_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/logfwd"
)

type memSender struct {
	mu   sync.Mutex
	html []string
	err  error
	hook func()
}

func (m *memSender) SendHTML(_ context.Context, html string) error {
	if m.hook != nil {
		m.hook()
	}
	m.mu.Lock()
	m.html = append(m.html, html)
	m.mu.Unlock()
	return m.err
}

func (m *memSender) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.html)
}

func (m *memSender) last() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.html) == 0 {
		return ""
	}
	return m.html[len(m.html)-1]
}

func waitCount(t *testing.T, m *memSender, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if m.count() >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("count=%d want %d", m.count(), want)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestHandler_ForwardsError(t *testing.T) {
	base := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := logfwd.New(base, logfwd.Options{MinLevel: slog.LevelError, Window: time.Minute})
	sender := &memSender{}
	h.SetSender(sender)
	log := slog.New(h)

	log.Info("skip me")
	log.Error("boom", "err", "x")
	waitCount(t, sender, 1)
	if !strings.Contains(sender.last(), "boom") || !strings.Contains(sender.last(), "🔴") {
		t.Fatalf("got %q", sender.last())
	}
}

func TestHandler_DedupeWindow(t *testing.T) {
	base := slog.NewJSONHandler(io.Discard, nil)
	h := logfwd.New(base, logfwd.Options{MinLevel: slog.LevelError, Window: time.Hour})
	sender := &memSender{}
	h.SetSender(sender)
	log := slog.New(h)

	log.Error("same")
	waitCount(t, sender, 1)
	log.Error("same")
	log.Error("same")
	time.Sleep(30 * time.Millisecond)
	if sender.count() != 1 {
		t.Fatalf("count=%d want 1 during window", sender.count())
	}
}

func TestHandler_LoopGuardDuringSend(t *testing.T) {
	base := slog.NewJSONHandler(io.Discard, nil)
	h := logfwd.New(base, logfwd.Options{MinLevel: slog.LevelError, Window: time.Millisecond})
	var nested atomic.Int32
	log := slog.New(h)
	sender := &memSender{hook: func() {
		// Simulate telegram send logging an error while forwarding.
		log.Error("telegram send failed during report")
		nested.Add(1)
	}}
	h.SetSender(sender)

	log.Error("original")
	waitCount(t, sender, 1)
	time.Sleep(50 * time.Millisecond)
	if nested.Load() != 1 {
		t.Fatalf("hook runs=%d", nested.Load())
	}
	// Nested error must not produce a second forward.
	if sender.count() != 1 {
		t.Fatalf("count=%d want 1 (loop guard)", sender.count())
	}
}

func TestHandler_NoSenderIsNoop(_ *testing.T) {
	base := slog.NewJSONHandler(io.Discard, nil)
	h := logfwd.New(base, logfwd.Options{MinLevel: slog.LevelError})
	log := slog.New(h)
	log.Error("nobody listening")
	time.Sleep(20 * time.Millisecond) // no panic, no sender
}

func TestParseLevel(t *testing.T) {
	_, on, err := logfwd.ParseLevel("off")
	if err != nil || on {
		t.Fatalf("off: on=%v err=%v", on, err)
	}
	lv, on, err := logfwd.ParseLevel("error")
	if err != nil || !on || lv != slog.LevelError {
		t.Fatalf("error: %v %v %v", lv, on, err)
	}
	lv, on, err = logfwd.ParseLevel("warn")
	if err != nil || !on || lv != slog.LevelWarn {
		t.Fatalf("warn: %v %v %v", lv, on, err)
	}
	if _, _, err := logfwd.ParseLevel("trace"); err == nil {
		t.Fatal("want error")
	}
}
