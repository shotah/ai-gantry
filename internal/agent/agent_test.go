package agent_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

type fakeCompleter struct {
	last  []provider.Message
	reply string
	err   error
}

func (f *fakeCompleter) Complete(_ context.Context, messages []provider.Message) (string, error) {
	f.last = append([]provider.Message(nil), messages...)
	return f.reply, f.err
}

type memHistory struct {
	mu   sync.Mutex
	data map[string][]session.Message
}

func newMemHistory() *memHistory {
	return &memHistory{data: make(map[string][]session.Message)}
}

func (m *memHistory) Messages(_ context.Context, id string) ([]session.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]session.Message(nil), m.data[id]...), nil
}

func (m *memHistory) Append(_ context.Context, id string, msgs ...session.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[id] = append(m.data[id], msgs...)
	return nil
}

func (m *memHistory) Reset(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, id)
	return nil
}

func (m *memHistory) Stats(ctx context.Context, id string) (int, int, error) {
	msgs, err := m.Messages(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	return len(msgs), session.EstTokens(msgs), nil
}

func TestAgent_Handle_PersonaAndHistory(t *testing.T) {
	fc := &fakeCompleter{reply: "hi back"}
	a, err := agent.New(agent.Options{
		Persona:   "you are tim",
		Completer: fc,
		Sessions:  newMemHistory(),
		Model:     "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := a.Handle(context.Background(), channel.Message{
		SessionID: "s1",
		Text:      "hello",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply != "hi back" {
		t.Fatalf("reply = %q", reply)
	}
	if len(fc.last) != 2 {
		t.Fatalf("messages = %d, want 2 (system+user)", len(fc.last))
	}
	if fc.last[0].Role != provider.RoleSystem || fc.last[0].Content != "you are tim" {
		t.Fatalf("system = %+v", fc.last[0])
	}

	fc.reply = "second"
	_, err = a.Handle(context.Background(), channel.Message{SessionID: "s1", Text: "again"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(fc.last) != 4 {
		t.Fatalf("messages = %d, want 4", len(fc.last))
	}
}

func TestAgent_NewAndStatus(t *testing.T) {
	fc := &fakeCompleter{reply: "ok"}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "x"})

	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/new@MyBot"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "session reset" {
		t.Fatalf("got %q", got)
	}

	status, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/status"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "history_messages=0") {
		t.Fatalf("status = %q", status)
	}
	if !strings.Contains(status, "model=m") {
		t.Fatalf("status = %q", status)
	}
}

func TestAgent_RequiresSessions(t *testing.T) {
	_, err := agent.New(agent.Options{Completer: &fakeCompleter{}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAgent_EmptyTextAndClear(t *testing.T) {
	fc := &fakeCompleter{reply: "ok"}
	h := newMemHistory()
	a, err := agent.New(agent.Options{Completer: fc, Sessions: h, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "   "})
	if err != nil || got != "" {
		t.Fatalf("empty text: %q %v", got, err)
	}
	_, _ = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "x"})
	got, err = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/clear"})
	if err != nil || got != "session reset" {
		t.Fatalf("clear: %q %v", got, err)
	}
}

func TestAgent_CompleteError(t *testing.T) {
	fc := &fakeCompleter{err: errors.New("llm down")}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAgent_CommandWithArgsNotSpecial(t *testing.T) {
	fc := &fakeCompleter{reply: "echo"}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/new please"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "echo" {
		t.Fatalf("got %q, want model reply for non-exact command", got)
	}
}
