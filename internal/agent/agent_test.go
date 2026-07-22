package agent_test

import (
	"context"
	"encoding/json"
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
	mu    sync.Mutex
	calls int
	fn    func(req provider.Request) (*provider.Result, error)
}

func (f *fakeCompleter) Complete(_ context.Context, req provider.Request) (*provider.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fn != nil {
		return f.fn(req)
	}
	return &provider.Result{Content: "ok"}, nil
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

type fakeTools struct {
	defs  []provider.ToolDef
	calls []string
	err   error
	out   string
}

func (f *fakeTools) Tools() []provider.ToolDef { return f.defs }
func (f *fakeTools) ToolCount() int            { return len(f.defs) }
func (f *fakeTools) Call(_ context.Context, name string, _ json.RawMessage) (string, error) {
	f.calls = append(f.calls, name)
	if f.err != nil {
		return "", f.err
	}
	if f.out != "" {
		return f.out, nil
	}
	return "tool-ok", nil
}

func TestAgent_Handle_PersonaAndHistory(t *testing.T) {
	var last provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		last = req
		return &provider.Result{Content: "hi back"}, nil
	}}
	a, err := agent.New(agent.Options{
		Persona:   "you are tim",
		Completer: fc,
		Sessions:  newMemHistory(),
		Model:     "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s1", Text: "hello"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply != "hi back" {
		t.Fatalf("reply = %q", reply)
	}
	if len(last.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(last.Messages))
	}
}

func TestAgent_ToolLoop(t *testing.T) {
	n := 0
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		n++
		if n == 1 {
			if len(req.Tools) != 1 {
				t.Fatalf("tools=%d", len(req.Tools))
			}
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "demo__echo", Arguments: `{"x":1}`},
			}}, nil
		}
		// second call should include tool result
		hasTool := false
		for _, m := range req.Messages {
			if m.Role == provider.RoleTool && m.ToolCallID == "c1" {
				hasTool = true
			}
		}
		if !hasTool {
			t.Fatal("missing tool result message")
		}
		return &provider.Result{Content: "final"}, nil
	}}
	tools := &fakeTools{defs: []provider.ToolDef{{Name: "demo__echo", Parameters: map[string]any{"type": "object"}}}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        tools,
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "use tool"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "final" {
		t.Fatalf("%q", got)
	}
	if len(tools.calls) != 1 || tools.calls[0] != "demo__echo" {
		t.Fatalf("%v", tools.calls)
	}
}

func TestAgent_NewAndStatus(t *testing.T) {
	fc := &fakeCompleter{}
	tools := &fakeTools{defs: []provider.ToolDef{{Name: "a__b"}}}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Tools: tools, Model: "m"})
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
	if !strings.Contains(status, "history_messages=0") || !strings.Contains(status, "tools=1") {
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
	a, err := agent.New(agent.Options{Completer: &fakeCompleter{}, Sessions: newMemHistory(), Model: "m"})
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
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return nil, errors.New("llm down")
	}}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAgent_MaxToolIterations(t *testing.T) {
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return &provider.Result{ToolCalls: []provider.ToolCall{
			{ID: "c", Name: "demo__echo", Arguments: `{}`},
		}}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "demo__echo"}}},
		MaxToolIters: 2,
		Model:        "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "loop"})
	if err == nil || !strings.Contains(err.Error(), "TOOL_MAX_ITERATIONS") {
		t.Fatalf("err = %v", err)
	}
}
