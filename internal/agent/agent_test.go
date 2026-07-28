package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/memory"
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

func (m *memHistory) Summary(_ context.Context, _ string) (string, error) {
	return "", nil
}

type fakeTools struct {
	defs  []provider.ToolDef
	calls []string
	err   error
	out   string
}

func (f *fakeTools) Tools() []provider.ToolDef { return f.defs }

func (f *fakeTools) ToolCount() int { return len(f.defs) }

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

func TestAgent_Handle_MemoryHydration(t *testing.T) {
	ctx := context.Background()
	mem, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()
	if _, err := mem.Store(ctx, memory.KindPreference, "chris", "coaching tone"); err != nil {
		t.Fatal(err)
	}

	var last provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		last = req
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{
		Persona:   "you are tim",
		Completer: fc,
		Sessions:  newMemHistory(),
		Memory:    mem,
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "hello chris"}); err != nil {
		t.Fatal(err)
	}
	if len(last.Messages) < 4 {
		t.Fatalf("want persona + memory + anchor + user, got %d", len(last.Messages))
	}
	if !strings.Contains(last.Messages[0].Content, "Persona files") {
		t.Fatalf("persona missing precedence note: %q", last.Messages[0].Content)
	}
	// Hydration is volatile per-turn content: it must sit AFTER history (here,
	// directly before the temporal anchor + user message) so the stable prompt
	// prefix is cacheable across turns.
	n := len(last.Messages)
	if !strings.Contains(last.Messages[n-3].Content, "[memory]") {
		t.Fatalf("missing hydration before anchor: %q", last.Messages[n-3].Content)
	}
	if !strings.Contains(last.Messages[n-2].Content, "[current time]") {
		t.Fatalf("missing temporal anchor: %q", last.Messages[n-2].Content)
	}
}

// Qwen-style stall: thinking present, no answer, no tool call. The agent must
// nudge the model to act (once) instead of returning an empty reply.
func TestAgent_Handle_ThinkingOnlyGetsNudged(t *testing.T) {
	ctx := context.Background()
	var reqs []provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		reqs = append(reqs, req)
		if len(reqs) == 1 {
			return &provider.Result{Thinking: "plan: call get_events for Monday"}, nil
		}
		return &provider.Result{Content: "your Monday is clear"}, nil
	}}
	a, err := agent.New(agent.Options{
		Persona:   "you are tim",
		Completer: fc,
		Sessions:  newMemHistory(),
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "what's tomorrow?"})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "your Monday is clear" {
		t.Fatalf("reply = %q", reply)
	}
	if len(reqs) != 2 {
		t.Fatalf("completions = %d, want 2 (original + nudged)", len(reqs))
	}
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if last.Role != provider.RoleSystem ||
		!strings.Contains(last.Content, "only internal reasoning") ||
		!strings.Contains(last.Content, "plan: call get_events for Monday") {
		t.Fatalf("missing nudge message: %+v", last)
	}
}

// After one nudge, a second thinking-only stall must ERROR (so Telegram
// error reporting fires) instead of silently returning an empty reply.
func TestAgent_Handle_ThinkingOnlyAfterNudgeErrors(t *testing.T) {
	ctx := context.Background()
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Thinking: "still planning…"}, nil
	}}
	a, err := agent.New(agent.Options{
		Persona:   "you are tim",
		Completer: fc,
		Sessions:  newMemHistory(),
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Handle(ctx, channel.Message{SessionID: "s", Text: "calendar?"})
	if err == nil || !strings.Contains(err.Error(), "stalled after thinking") {
		t.Fatalf("err = %v, want stalled after thinking", err)
	}
}

// Prose that promises a tool ("I'll pull…") without tool_calls must be nudged
// once so the model actually invokes the function.
func TestAgent_Handle_ProseToolPromiseGetsNudged(t *testing.T) {
	ctx := context.Background()
	var reqs int
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		reqs++
		switch reqs {
		case 1:
			return &provider.Result{
				Content: "You got it! Let me pull your Garmin sleep data for last night.",
			}, nil
		case 2:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != provider.RoleSystem || !strings.Contains(last.Content, "did not emit a tool call") {
				t.Fatalf("missing tool-promise nudge: %+v", last)
			}
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "garmin__get_sleep", Arguments: `{}`},
			}}, nil
		default:
			return &provider.Result{Content: "Sleep score 80 — solid night."}, nil
		}
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "garmin__get_sleep", Parameters: map[string]any{"type": "object"}}},
		out:  `{"sleepScore":80}`,
	}
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
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "how was my sleep last night?"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "Sleep score 80") {
		t.Fatalf("reply = %q", reply)
	}
	if len(tools.calls) != 1 || tools.calls[0] != "garmin__get_sleep" {
		t.Fatalf("tools = %v", tools.calls)
	}
	if reqs != 3 {
		t.Fatalf("completions = %d, want 3", reqs)
	}
}

// After a successful tool call, Qwen often puts the user-facing answer only in
// Thinking. Promote that CoT instead of nudging into another stall/ERROR.
func TestAgent_Handle_ThinkingOnlyAfterToolsPromotes(t *testing.T) {
	ctx := context.Background()
	var reqs int
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		reqs++
		if reqs == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "garmin__get_sleep", Arguments: `{"date":"2026-07-28"}`},
			}}, nil
		}
		return &provider.Result{Thinking: "✅ Sleep score 78 — about 5h 36m total."}, nil
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "garmin__get_sleep", Parameters: map[string]any{"type": "object"}}},
		out:  `{"sleepScore":78,"totalSleepSeconds":20160}`,
	}
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
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "how was my sleep?"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "Sleep score 78") {
		t.Fatalf("reply = %q, want promoted thinking", reply)
	}
	if reqs != 2 {
		t.Fatalf("completions = %d, want 2 (tool + think-only)", reqs)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("tool calls = %v", tools.calls)
	}
}

func TestAgent_Handle_PersonaAndHistory(t *testing.T) {
	loc := time.FixedZone("test", -8*3600)
	hist := newMemHistory()
	var last provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		last = req
		return &provider.Result{Content: "hi back"}, nil
	}}
	a, err := agent.New(agent.Options{
		Persona:   "you are tim",
		Completer: fc,
		Sessions:  hist,
		Model:     "test-model",
		Location:  loc,
		TZName:    "America/Los_Angeles",
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
	// persona + temporal anchor + user
	if len(last.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(last.Messages))
	}
	if !strings.Contains(last.Messages[1].Content, "[current time]") {
		t.Fatalf("missing temporal anchor: %q", last.Messages[1].Content)
	}
	if !strings.Contains(last.Messages[1].Content, "America/Los_Angeles") {
		t.Fatalf("temporal missing tz: %q", last.Messages[1].Content)
	}
	// Anchor must not be persisted into session history.
	stored, err := hist.Messages(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range stored {
		if strings.Contains(m.Content, "[current time]") {
			t.Fatalf("temporal anchor leaked into history: %+v", m)
		}
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

	listed, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/tools"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "a__b") || !strings.Contains(listed, "server=a") {
		t.Fatalf("tools = %q", listed)
	}

	a.SetPersona("reloaded")
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
