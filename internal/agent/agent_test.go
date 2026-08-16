package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/mcp"
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
	mu      sync.Mutex
	data    map[string][]session.Message
	summary map[string]string
}

func newMemHistory() *memHistory {
	return &memHistory{data: make(map[string][]session.Message), summary: make(map[string]string)}
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
	delete(m.summary, id)
	return nil
}

func (m *memHistory) Stats(ctx context.Context, id string) (int, int, error) {
	msgs, err := m.Messages(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	return len(msgs), session.EstTokens(msgs), nil
}

func (m *memHistory) Summary(_ context.Context, id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.summary[id], nil
}

func (m *memHistory) setSummary(id, s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.summary[id] = s
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
		Persona:   "you are SAM",
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
		t.Fatalf("want persona + memory + user + anchor, got %d", len(last.Messages))
	}
	if !strings.Contains(last.Messages[0].Content, "Persona files") {
		t.Fatalf("persona missing precedence note: %q", last.Messages[0].Content)
	}
	// Hydration is volatile per-turn content: it must sit AFTER history so the
	// stable prompt prefix is cacheable across turns. Temporal clock is a
	// footer after the user message (intent first, clock as reference).
	n := len(last.Messages)
	if !strings.Contains(last.Messages[n-3].Content, "[memory]") {
		t.Fatalf("missing hydration before user: %q", last.Messages[n-3].Content)
	}
	if last.Messages[n-2].Role != provider.RoleUser {
		t.Fatalf("want user before temporal footer, got %q", last.Messages[n-2].Role)
	}
	if !strings.Contains(last.Messages[n-1].Content, "[current time]") {
		t.Fatalf("missing temporal footer: %q", last.Messages[n-1].Content)
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
		Persona:   "you are SAM",
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
		Persona:   "you are SAM",
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
			if last.Role != provider.RoleSystem || !strings.Contains(last.Content, "no tool call was made") {
				t.Fatalf("missing tool-promise nudge: %+v", last)
			}
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "garmin__sleep_get", Arguments: `{}`},
			}}, nil
		default:
			return &provider.Result{Content: "Sleep score 80 — solid night."}, nil
		}
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "garmin__sleep_get", Parameters: map[string]any{"type": "object"}}},
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
	if len(tools.calls) != 1 || tools.calls[0] != "garmin__sleep_get" {
		t.Fatalf("tools = %v", tools.calls)
	}
	if reqs != 3 {
		t.Fatalf("completions = %d, want 3", reqs)
	}
}

// A fabricated success claim ("I've created…") with zero tool calls must get
// the same nudge as a promise — the model then makes the real call.
func TestAgent_Handle_FakeSuccessClaimGetsNudged(t *testing.T) {
	ctx := context.Background()
	var reqs int
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		reqs++
		switch reqs {
		case 1:
			return &provider.Result{
				Content: "You got it! I've created a new task list in Google Tasks for you.",
			}, nil
		case 2:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != provider.RoleSystem || !strings.Contains(last.Content, "nothing actually happened") {
				t.Fatalf("missing fake-success nudge: %+v", last)
			}
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "google__tasks_create_tasklist", Arguments: `{"title":"Groceries"}`},
			}}, nil
		default:
			return &provider.Result{Content: "Created the Groceries list."}, nil
		}
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "google__tasks_create_tasklist", Parameters: map[string]any{"type": "object"}}},
		out:  `{"id":"list1","title":"Groceries"}`,
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
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "Can you create a Google task list"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "Groceries") {
		t.Fatalf("reply = %q", reply)
	}
	if len(tools.calls) != 1 || tools.calls[0] != "google__tasks_create_tasklist" {
		t.Fatalf("tools = %v", tools.calls)
	}
	if reqs != 3 {
		t.Fatalf("completions = %d, want 3", reqs)
	}
}

// A cron live-data job that drafts the digest with fake numbers (no tool
// theater cues) must be nudged so the model actually calls Garmin/etc.
func TestAgent_Handle_CronLiveDataReportWithoutToolsGetsNudged(t *testing.T) {
	ctx := context.Background()
	var reqs int
	var firstReq provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		reqs++
		switch reqs {
		case 1:
			firstReq = req
			return &provider.Result{
				Content: "Unified Morning Audit\n| Sleep | 81 |\n| HRV | 62 |\nReady to train.",
			}, nil
		case 2:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != provider.RoleSystem || !strings.Contains(last.Content, "scheduled job needs live data") {
				t.Fatalf("missing cron live-data nudge: %+v", last)
			}
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "garmin__sleep_get", Arguments: `{}`},
			}}, nil
		default:
			return &provider.Result{Content: "Sleep score 74 — from Garmin."}, nil
		}
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "garmin__sleep_get", Parameters: map[string]any{"type": "object"}}},
		out:  `{"sleepScore":74}`,
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
	text := cron.JobUserPrefix + "Fetch Garmin sleep and present the Unified Morning Audit."
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "Sleep score 74") || !strings.Contains(reply, "— tools: garmin__sleep_get") {
		t.Fatalf("reply = %q", reply)
	}
	if len(tools.calls) != 1 || tools.calls[0] != "garmin__sleep_get" {
		t.Fatalf("tools = %v", tools.calls)
	}
	if reqs != 3 {
		t.Fatalf("completions = %d, want 3", reqs)
	}
	foundNote := false
	for _, m := range firstReq.Messages {
		if strings.Contains(m.Content, "Scheduled turn: if this job needs live data") {
			foundNote = true
			break
		}
	}
	if !foundNote {
		t.Fatal("missing cron tool-first system note on first completion")
	}
}

// A no-tool reminder must not be forced into a tool call just because it
// arrived on the cron path.
func TestAgent_Handle_CronReminderWithoutLiveDataNotNudged(t *testing.T) {
	ctx := context.Background()
	var reqs int
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		reqs++
		return &provider.Result{Content: "Time to submit your timecard."}, nil
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "garmin__sleep_get", Parameters: map[string]any{"type": "object"}}},
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
	text := cron.JobUserPrefix + "Remind me to submit my timecard."
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "Time to submit your timecard." {
		t.Fatalf("reply = %q", reply)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("tools = %v, want none", tools.calls)
	}
	if reqs != 1 {
		t.Fatalf("completions = %d, want 1 (no nudge)", reqs)
	}
	if strings.Contains(reply, "— tools:") {
		t.Fatalf("reminder should not get a tool footer: %q", reply)
	}
}

// [silent] after a live-data pull must not grow a tools footer (runner skips the push).
func TestAgent_Handle_CronSilentReplySkipsToolFooter(t *testing.T) {
	ctx := context.Background()
	var reqs int
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		reqs++
		if reqs == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "garmin__sleep_get", Arguments: `{}`},
			}}, nil
		}
		return &provider.Result{Content: cron.SilentToken + "\nall-clear"}, nil
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "garmin__sleep_get", Parameters: map[string]any{"type": "object"}}},
		out:  `{"sleepScore":81}`,
	}
	hist := newMemHistory()
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     hist,
		Tools:        tools,
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := cron.JobUserPrefix + "Fetch Garmin sleep. If all-clear, reply [silent]."
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if !cron.IsSilentReply(reply) {
		t.Fatalf("reply = %q, want silent", reply)
	}
	if strings.Contains(reply, "— tools:") {
		t.Fatalf("silent reply must not get a tool footer: %q", reply)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("tools = %v, want one pull", tools.calls)
	}
	msgs, err := hist.Messages(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) < 2 || !cron.IsSilentReply(msgs[len(msgs)-1].Content) {
		t.Fatalf("session should keep the silent turn, got %+v", msgs)
	}
}

// After a live-data nudge, a second no-tool draft must not ship invented metrics.
func TestAgent_Handle_CronLiveDataReportAfterNudgeRefused(t *testing.T) {
	ctx := context.Background()
	var reqs int
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		reqs++
		return &provider.Result{
			Content: "Unified Morning Audit\n| Sleep | 74 |\n| HRV | 44 |",
		}, nil
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "garmin__sleep_get", Parameters: map[string]any{"type": "object"}}},
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
	text := cron.JobUserPrefix + "Fetch Garmin sleep and present the Unified Morning Audit."
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "I won't invent metrics") {
		t.Fatalf("reply = %q, want refuse", reply)
	}
	if strings.Contains(reply, "Sleep | 74") {
		t.Fatalf("shipped invented metrics: %q", reply)
	}
	if !strings.Contains(reply, "— tools: (none)") {
		t.Fatalf("reply = %q, want tools-none footer", reply)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("tools = %v", tools.calls)
	}
	if reqs != 2 {
		t.Fatalf("completions = %d, want 2 (draft + nudged draft)", reqs)
	}
}

// Prior scheduled digests must not appear in the next cron prompt (few-shot bait).
func TestAgent_Handle_CronOmitsPriorCronHistory(t *testing.T) {
	ctx := context.Background()
	hist := newMemHistory()
	if err := hist.Append(ctx, "s",
		session.Message{Role: session.RoleUser, Content: "hey"},
		session.Message{Role: session.RoleAssistant, Content: "hi there"},
		session.Message{Role: session.RoleUser, Content: cron.JobUserPrefix + "Fetch Garmin sleep"},
		session.Message{Role: session.RoleAssistant, Content: "UNIFIED MORNING AUDIT\nSleep Score: 81"},
	); err != nil {
		t.Fatal(err)
	}
	var first provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		if first.Messages == nil {
			first = req
		}
		return &provider.Result{Content: "Time to submit your timecard."}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     hist,
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "garmin__sleep_get", Parameters: map[string]any{"type": "object"}}}},
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := cron.JobUserPrefix + "Remind me to submit my timecard."
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: text}); err != nil {
		t.Fatal(err)
	}
	blob := ""
	for _, m := range first.Messages {
		blob += m.Content + "\n"
	}
	if strings.Contains(blob, "Sleep Score: 81") || strings.Contains(blob, "UNIFIED MORNING AUDIT") {
		t.Fatalf("prior cron audit leaked into prompt:\n%s", blob)
	}
	if !strings.Contains(blob, "hey") || !strings.Contains(blob, "hi there") {
		t.Fatalf("interactive history was dropped:\n%s", blob)
	}
}

// After tools fail, prose that defers ("give me a moment…") without another
// tool_call must be nudged — otherwise the human is left hanging.
func TestAgent_Handle_PostToolDeferralGetsNudged(t *testing.T) {
	ctx := context.Background()
	var reqs int
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		reqs++
		switch reqs {
		case 1:
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "google__sheets_read_values", Arguments: `{"spreadsheet_id":"x"}`},
			}}, nil
		case 2:
			return &provider.Result{
				Content: "I am going to try to access that specific sheet now using your ID. Give me one moment while I confirm the connection.",
			}, nil
		case 3:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != provider.RoleSystem || !strings.Contains(last.Content, "left hanging") {
				t.Fatalf("missing post-tool deferral nudge: %+v", last)
			}
			return &provider.Result{Content: "I give up — google__sheets_read_values failed with auth error."}, nil
		default:
			t.Fatalf("unexpected completion #%d", reqs)
			return nil, nil
		}
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "google__sheets_read_values", Parameters: map[string]any{"type": "object"}}},
		err:  errors.New("unauthorized"),
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
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "read this sheet"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "I give up") {
		t.Fatalf("reply = %q", reply)
	}
	if reqs != 3 {
		t.Fatalf("completions = %d, want 3", reqs)
	}
}

// A second deferral after the nudge must not ship "give me a moment" as the reply.
func TestAgent_Handle_PostToolDeferralAfterNudgeGivesUp(t *testing.T) {
	ctx := context.Background()
	var reqs int
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		reqs++
		if reqs == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "google__sheets_read_values", Arguments: `{"spreadsheet_id":"x"}`},
			}}, nil
		}
		return &provider.Result{
			Content: "Give me one moment while I confirm the connection.",
		}, nil
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "google__sheets_read_values", Parameters: map[string]any{"type": "object"}}},
		err:  errors.New("unauthorized"),
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
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "read this sheet"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "couldn't finish") {
		t.Fatalf("reply = %q, want forced give-up", reply)
	}
	if reqs != 3 {
		t.Fatalf("completions = %d, want 3 (tool + defer + nudged defer)", reqs)
	}
}

// Honest post-tool failure that mentions the tool name must NOT be treated as a
// promise ( "__" in google__… ) and re-nudged.
func TestAgent_Handle_PostToolHonestFailureAccepted(t *testing.T) {
	ctx := context.Background()
	var reqs int
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		reqs++
		if reqs == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "google__sheets_read_values", Arguments: `{"spreadsheet_id":"x"}`},
			}}, nil
		}
		return &provider.Result{
			Content: "I give up — google__sheets_read_values failed with unauthorized.",
		}, nil
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "google__sheets_read_values", Parameters: map[string]any{"type": "object"}}},
		err:  errors.New("unauthorized"),
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
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "read this sheet"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "I give up") {
		t.Fatalf("reply = %q", reply)
	}
	if reqs != 2 {
		t.Fatalf("completions = %d, want 2 (no false nudge)", reqs)
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
				{ID: "c1", Name: "garmin__sleep_get", Arguments: `{"date":"2026-07-28"}`},
			}}, nil
		}
		return &provider.Result{Thinking: "✅ Sleep score 78 — about 5h 36m total."}, nil
	}}
	tools := &fakeTools{
		defs: []provider.ToolDef{{Name: "garmin__sleep_get", Parameters: map[string]any{"type": "object"}}},
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
		Persona:   "you are SAM",
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
	// persona + user + temporal footer
	if len(last.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(last.Messages))
	}
	if last.Messages[1].Role != provider.RoleUser {
		t.Fatalf("want user before temporal footer, got %q", last.Messages[1].Role)
	}
	if !strings.Contains(last.Messages[2].Content, "[current time]") {
		t.Fatalf("missing temporal footer: %q", last.Messages[2].Content)
	}
	if !strings.Contains(last.Messages[2].Content, "America/Los_Angeles") {
		t.Fatalf("temporal missing tz: %q", last.Messages[2].Content)
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

func TestAgent_Handle_UsesUserMarkdownTimezone(t *testing.T) {
	var last provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		last = req
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{
		Persona:   "- **Timezone:** America/Los_Angeles\n- **Name:** Chris",
		Completer: fc,
		Sessions:  newMemHistory(),
		Model:     "m",
		Location:  time.UTC,
		TZName:    "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "what time is it"}); err != nil {
		t.Fatal(err)
	}
	n := len(last.Messages)
	if n < 1 {
		t.Fatal("no messages")
	}
	got := last.Messages[n-1].Content
	if !strings.Contains(got, "America/Los_Angeles") {
		t.Fatalf("USER.md tz ignored: %q", got)
	}
	if strings.Contains(got, "UTC-") || strings.Contains(got, "UTC+") {
		t.Fatalf("still labeled UTC: %q", got)
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
	if !strings.Contains(status, "history_messages=0") ||
		!strings.Contains(status, "tools=1") ||
		!strings.Contains(status, "schema_est_tokens=") ||
		!strings.Contains(status, "history_est_tokens=") ||
		!strings.Contains(status, "turns=1") {
		t.Fatalf("status = %q", status)
	}

	listed, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/tools"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "a__b") ||
		!strings.Contains(listed, "server=a") ||
		!strings.Contains(listed, "schema_est_tokens≈") ||
		!strings.Contains(listed, "by server:") {
		t.Fatalf("tools = %q", listed)
	}

	a.SetPersona("reloaded")
}

func TestAgent_Perf(t *testing.T) {
	fc := &fakeCompleter{}
	a, err := agent.New(agent.Options{
		Completer: fc,
		Sessions:  newMemHistory(),
		Model:     "m",
		StartedAt: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	empty, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/perf"})
	if err != nil {
		t.Fatal(err)
	}
	if empty != "no turns yet" {
		t.Fatalf("empty perf = %q", empty)
	}

	if _, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "second"}); err != nil {
		t.Fatal(err)
	}

	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/perf"})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Fatalf("perf = %q", got)
	}
	if !strings.HasPrefix(lines[0], "perf — last 2 turns") {
		t.Fatalf("header = %q", lines[0])
	}
	// Newest first: #2 then #1; only the first turn after boot is cold.
	if !strings.HasPrefix(lines[1], "#2 ") || strings.Contains(lines[1], "← cold") {
		t.Fatalf("newest = %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "#1 ") || !strings.Contains(lines[2], "← cold") {
		t.Fatalf("oldest = %q", lines[2])
	}

	// Cap: fill past the ring size; /perf must keep only the newest 12.
	for i := 0; i < 15; i++ {
		if _, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	capped, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/perf"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capped, "last 12 turns") {
		t.Fatalf("capped header missing: %q", capped)
	}
	capLines := strings.Split(capped, "\n")
	if len(capLines) != 13 { // header + 12
		t.Fatalf("want 13 lines, got %d: %q", len(capLines), capped)
	}
	if !strings.HasPrefix(capLines[1], "#17 ") {
		t.Fatalf("newest after cap = %q", capLines[1])
	}
	if strings.Contains(capped, "#1 ") {
		t.Fatalf("ring should have dropped early turns: %q", capped)
	}

	status, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/status"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "turns=17") {
		t.Fatalf("status turns = %q", status)
	}
}

func TestAgent_MemStats(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if _, err := store.Store(ctx, memory.KindFact, "name", "chris"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Store(ctx, memory.KindPreference, "coffee", "black"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Store(ctx, memory.KindEpisode, "chat", "talked about coffee"); err != nil {
		t.Fatal(err)
	}

	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  newMemHistory(),
		Memory:    store,
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "/memstats"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "memory: 3 rows") ||
		!strings.Contains(got, "fact=1") ||
		!strings.Contains(got, "preference=1") ||
		!strings.Contains(got, "episode=1") ||
		!strings.Contains(got, "consolidation: off") ||
		!strings.Contains(got, "(WAL)") {
		t.Fatalf("memstats = %q", got)
	}

	disabled, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  newMemHistory(),
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	off, err := disabled.Handle(ctx, channel.Message{SessionID: "s", Text: "/memstats"})
	if err != nil || off != "memory: disabled" {
		t.Fatalf("disabled = %q %v", off, err)
	}
}

func TestAgent_Tokens(t *testing.T) {
	hist := newMemHistory()
	hist.setSummary("s", "chris likes espresso")
	if err := hist.Append(context.Background(), "s",
		session.Message{Role: session.RoleUser, Content: "hi"},
		session.Message{Role: session.RoleAssistant, Content: "hello"},
	); err != nil {
		t.Fatal(err)
	}

	a, err := agent.New(agent.Options{
		Persona:   "you are a test persona with some padding text",
		Completer: &fakeCompleter{},
		Sessions:  hist,
		Tools:     &fakeTools{defs: []provider.ToolDef{{Name: "a__b", Description: "demo"}}},
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/tokens"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tokens (chars/4 estimates)", "persona", "summary", "history", "hydration", "schemas", "standing", "(2 msgs)", "(off)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tokens missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "summary") || strings.Contains(got, "  summary     0\n") {
		t.Fatalf("expected non-zero summary estimate:\n%s", got)
	}
}

func TestAgent_Help(t *testing.T) {
	a, err := agent.New(agent.Options{Completer: &fakeCompleter{}, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/help"})
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"/new", "/cancel", "/status", "/tools", "/perf", "/memstats", "/toolstats", "/tokens", "/auth", "/help"} {
		if !strings.Contains(got, cmd) {
			t.Fatalf("help missing %s: %q", cmd, got)
		}
	}
}

func TestAgent_ToolStats(t *testing.T) {
	none, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  newMemHistory(),
		Tools:     &fakeTools{defs: []provider.ToolDef{{Name: "x__y"}}},
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := none.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/toolstats"})
	if err != nil || got != "no tool calls yet" {
		t.Fatalf("empty = %q %v", got, err)
	}

	manifest := filepath.Join(t.TempDir(), "mcp.toml")
	if err := os.WriteFile(manifest, []byte(`
[[server]]
name = "fast"
command = "unused"

[[server]]
name = "slow"
command = "unused"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: manifest,
		Dial: func(_ context.Context, spec mcp.ServerSpec, _ io.Writer) (mcp.Conn, error) {
			switch spec.Name {
			case "fast":
				return &toolStatsConn{name: "ping"}, nil
			case "slow":
				return &toolStatsConn{name: "work", failOnce: true}, nil
			default:
				return nil, errors.New("unknown server")
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	// Seed counters via the host (same path Call uses).
	_, _ = host.Call(context.Background(), "fast__ping", json.RawMessage(`{}`))
	_, _ = host.Call(context.Background(), "fast__ping", json.RawMessage(`{}`))
	_, _ = host.Call(context.Background(), "slow__work", json.RawMessage(`{}`))

	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  newMemHistory(),
		Tools:     host,
		Model:     "m",
		StartedAt: time.Now().Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/toolstats"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tool stats — 3 calls since boot") ||
		!strings.Contains(out, "fast__ping") ||
		!strings.Contains(out, "slow__work") ||
		!strings.Contains(out, "✓2 ✗0") ||
		!strings.Contains(out, "✓0 ✗1") ||
		!strings.Contains(out, "repairs:") {
		t.Fatalf("toolstats = %q", out)
	}
	// Sorted by total time — both lines present; order may tie on near-zero fake durations.
	idxFast := strings.Index(out, "fast__ping")
	idxSlow := strings.Index(out, "slow__work")
	if idxFast < 0 || idxSlow < 0 {
		t.Fatalf("missing tools in %q", out)
	}
}

// toolStatsConn is a minimal mcp.Conn for /toolstats slash tests.
type toolStatsConn struct {
	name     string
	failOnce bool
}

func (c *toolStatsConn) ListTools(context.Context) ([]mcp.Tool, error) {
	return []mcp.Tool{{OriginalName: c.name, InputSchema: map[string]any{"type": "object"}}}, nil
}

func (c *toolStatsConn) CallTool(context.Context, string, map[string]any) (string, error) {
	if c.failOnce {
		c.failOnce = false
		return "", errors.New("invalid argument: boom")
	}
	return "ok", nil
}

func (c *toolStatsConn) Close() error { return nil }

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

// A provider that keeps returning tool_calls even on the landing call (where
// tools are withheld) must still terminate with the budget error.
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

// Exhausting the tool budget must land gracefully: the final call runs without
// tools plus a wrap-up note, the model's text becomes the reply, and the turn
// is persisted instead of erroring out and dropping all the tool work.
func TestAgent_MaxToolIterations_GracefulLanding(t *testing.T) {
	var reqs []provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		reqs = append(reqs, req)
		if len(req.Tools) > 0 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c", Name: "demo__echo", Arguments: `{}`},
			}}, nil
		}
		return &provider.Result{Content: "ran out of tool budget; here is what I found"}, nil
	}}
	hist := newMemHistory()
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     hist,
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "demo__echo"}}},
		MaxToolIters: 2,
		Model:        "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "loop"})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "ran out of tool budget; here is what I found" {
		t.Fatalf("reply = %q", reply)
	}
	if len(reqs) != 3 {
		t.Fatalf("model calls = %d, want 2 tool rounds + landing", len(reqs))
	}
	landing := reqs[len(reqs)-1]
	last := landing.Messages[len(landing.Messages)-1]
	if last.Role != provider.RoleSystem || !strings.Contains(last.Content, "Tool budget exhausted") {
		t.Fatalf("landing call missing budget note: %+v", last)
	}
	msgs, err := hist.Messages(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("history = %d messages, want persisted user + assistant", len(msgs))
	}
}

// Past ~70% of the budget the model is told how many rounds remain, once.
func TestAgent_MaxToolIterations_WarnsNearBudget(t *testing.T) {
	var reqs []provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		reqs = append(reqs, req)
		if len(reqs) < 5 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c", Name: "demo__echo", Arguments: `{}`},
			}}, nil
		}
		return &provider.Result{Content: "done"}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "demo__echo"}}},
		MaxToolIters: 5, // warn after round 3 (70% floor), 2 rounds remain
		Model:        "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "loop"}); err != nil {
		t.Fatal(err)
	}
	countWarnings := func(req provider.Request) int {
		n := 0
		for _, m := range req.Messages {
			if m.Role == provider.RoleSystem && strings.Contains(m.Content, "[system] Tool budget:") {
				n++
			}
		}
		return n
	}
	if n := countWarnings(reqs[2]); n != 0 {
		t.Fatalf("round 3 request already has %d warnings", n)
	}
	final := reqs[len(reqs)-1]
	if n := countWarnings(final); n != 1 {
		t.Fatalf("final request has %d budget warnings, want exactly 1", n)
	}
	if !strings.Contains(warningText(final), "2 remain") {
		t.Fatalf("warning text = %q, want remaining rounds", warningText(final))
	}
}

func warningText(req provider.Request) string {
	for _, m := range req.Messages {
		if m.Role == provider.RoleSystem && strings.Contains(m.Content, "[system] Tool budget:") {
			return m.Content
		}
	}
	return ""
}

func TestAgent_StripFillersOnOldHistory(t *testing.T) {
	var seen []provider.Message
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		seen = append([]provider.Message(nil), req.Messages...)
		return &provider.Result{Content: "ok"}, nil
	}}
	hist := newMemHistory()
	for i := 0; i < 6; i++ {
		if err := hist.Append(context.Background(), "s",
			session.Message{Role: session.RoleUser, Content: "the calendar on Tuesday"},
			session.Message{Role: session.RoleAssistant, Content: "the day is clear"},
		); err != nil {
			t.Fatal(err)
		}
	}
	a, err := agent.New(agent.Options{
		Completer:           fc,
		Sessions:            hist,
		Model:               "m",
		HistoryStripFillers: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, m := range seen {
		if m.Role == provider.RoleUser {
			users = append(users, m.Content)
		}
	}
	if len(users) < 2 {
		t.Fatalf("user msgs=%v", users)
	}
	if users[0] == "the calendar on Tuesday" || strings.Contains(users[0], " the ") {
		t.Fatalf("oldest history not stripped: %q", users[0])
	}
	if users[len(users)-1] != "hi" {
		t.Fatalf("current user = %q", users[len(users)-1])
	}
	stored, _ := hist.Messages(context.Background(), "s")
	if stored[0].Content != "the calendar on Tuesday" {
		t.Fatalf("SQLite rewritten: %q", stored[0].Content)
	}
}
