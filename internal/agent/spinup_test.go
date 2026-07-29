package agent_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

// statusWriter records every status line set on it, including the "" that
// clears one, so tests can assert the notice is taken back down.
type statusWriter struct {
	memWriter
	mu   sync.Mutex
	sets []string
}

func (s *statusWriter) UpdateStatus(_ context.Context, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sets = append(s.sets, note)
	return nil
}

func (s *statusWriter) statuses() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sets...)
}

// newSpinupAgent builds an agent whose only tool-free reply is "ok".
func newSpinupAgent(t *testing.T, notice time.Duration, delay time.Duration) *agent.Agent {
	t.Helper()
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		time.Sleep(delay)
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Model:        "m",
		MaxToolIters: 5,
		SpinupNotice: notice,
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func handleOnce(t *testing.T, a *agent.Agent, w channel.ReplyWriter, text string) string {
	t.Helper()
	ctx := channel.WithReplyWriter(context.Background(), w)
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

// The first turn of the process is known-cold, so it must not wait for the
// threshold — here the threshold is an hour and the note still lands. It must
// also be cleared once the model answers.
func TestAgent_SpinupNotice_ColdTurnPostsThenClears(t *testing.T) {
	a := newSpinupAgent(t, time.Hour, 0)
	w := &statusWriter{}
	handleOnce(t, a, w, "hey")

	sets := w.statuses()
	if len(sets) != 2 {
		t.Fatalf("statuses = %q, want notice + clear", sets)
	}
	if !strings.Contains(sets[0], "spinning up") {
		t.Fatalf("notice = %q", sets[0])
	}
	if sets[1] != "" {
		t.Fatalf("statuses = %q, want the notice cleared", sets)
	}
}

// Once warm, a turn that answers quickly should stay silent.
func TestAgent_SpinupNotice_WarmFastTurnStaysQuiet(t *testing.T) {
	a := newSpinupAgent(t, time.Hour, 0)
	handleOnce(t, a, &statusWriter{}, "warm it up")

	w := &statusWriter{}
	handleOnce(t, a, w, "and again")
	if sets := w.statuses(); len(sets) != 0 {
		t.Fatalf("statuses = %q, want none on a warm fast turn", sets)
	}
}

// A warm turn that stalls past the threshold gets the generic notice — this is
// the prompt-cache-miss case, which no provider API exposes.
func TestAgent_SpinupNotice_WarmSlowTurnPostsAfterThreshold(t *testing.T) {
	a := newSpinupAgent(t, 10*time.Millisecond, 250*time.Millisecond)
	handleOnce(t, a, &statusWriter{}, "warm it up")

	w := &statusWriter{}
	handleOnce(t, a, w, "slow one")
	sets := w.statuses()
	if len(sets) != 2 || !strings.Contains(sets[0], "working on it") || sets[1] != "" {
		t.Fatalf("statuses = %q, want slow notice + clear", sets)
	}
}

func TestAgent_SpinupNotice_ZeroDisables(t *testing.T) {
	a := newSpinupAgent(t, 0, 50*time.Millisecond)
	w := &statusWriter{}
	handleOnce(t, a, w, "hey")
	if sets := w.statuses(); len(sets) != 0 {
		t.Fatalf("statuses = %q, want none when disabled", sets)
	}
}

// Writers without UpdateStatus (stdio/slack/discord) must be unaffected.
func TestAgent_SpinupNotice_PlainWriterUnaffected(t *testing.T) {
	a := newSpinupAgent(t, time.Hour, 0)
	if got := handleOnce(t, a, &memWriter{}, "hey"); got != "ok" {
		t.Fatalf("reply = %q, want the plain reply", got)
	}
}
