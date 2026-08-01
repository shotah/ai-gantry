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
)

// progressWriter is a ReplyWriter that also records tool trace lines.
type progressWriter struct {
	memWriter
	mu    sync.Mutex
	notes []string
}

func (p *progressWriter) UpdateProgress(_ context.Context, note string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.notes = append(p.notes, note)
	return nil
}

func (p *progressWriter) traced() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.notes...)
}

func TestAgent_ToolProgress_TracesEachCall(t *testing.T) {
	n := 0
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		n++
		if n == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "garmin__activities_list", Arguments: `{}`},
			}}, nil
		}
		return &provider.Result{Content: "21mi, nice"}, nil
	}}
	w := &progressWriter{}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "garmin__activities_list"}}, out: strings.Repeat("x", 4100)},
		Model:        "m",
		MaxToolIters: 5,
		ToolTrace:    agent.ToolTraceFull,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := channel.WithReplyWriter(context.Background(), w)
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "yesterday's ride?"}); err != nil {
		t.Fatal(err)
	}
	notes := w.traced()
	if len(notes) != 2 {
		t.Fatalf("notes = %v, want start + done", notes)
	}
	if !strings.Contains(notes[0], "garmin__activities_list") {
		t.Fatalf("start note = %q", notes[0])
	}
	// Duration is timing-dependent; the size summary is not.
	if !strings.Contains(notes[1], "4.1k chars") {
		t.Fatalf("done note = %q", notes[1])
	}
}

func TestAgent_ToolProgress_CompactMode(t *testing.T) {
	n := 0
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		n++
		switch n {
		case 1:
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "flights__dates_search", Arguments: `{}`},
				{ID: "c2", Name: "flights__offers_search", Arguments: `{}`},
			}}, nil
		case 2:
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c3", Name: "flights__link_format", Arguments: `{}`},
			}}, nil
		default:
			return &provider.Result{Content: "here are options"}, nil
		}
	}}
	w := &progressWriter{}
	a, err := agent.New(agent.Options{
		Completer: fc,
		Sessions:  newMemHistory(),
		Tools: &fakeTools{defs: []provider.ToolDef{
			{Name: "flights__dates_search"},
			{Name: "flights__offers_search"},
			{Name: "flights__link_format"},
		}, err: errors.New("boom")},
		Model:        "m",
		MaxToolIters: 5,
		ToolTrace:    agent.ToolTraceCompact,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := channel.WithReplyWriter(context.Background(), w)
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "flights?"}); err != nil {
		t.Fatal(err)
	}
	notes := w.traced()
	want := []string{"Making Calls:", "✗", "✗", "Making Calls:", "✗"}
	if len(notes) != len(want) {
		t.Fatalf("notes = %v, want %v", notes, want)
	}
	for i := range want {
		if notes[i] != want[i] {
			t.Fatalf("notes[%d] = %q, want %q (all=%v)", i, notes[i], want[i], notes)
		}
		if strings.Contains(notes[i], "flights__") {
			t.Fatalf("compact mode leaked tool name: %q", notes[i])
		}
	}
}

func TestAgent_ToolProgress_OffMode(t *testing.T) {
	n := 0
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		n++
		if n == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "demo__echo", Arguments: `{}`},
			}}, nil
		}
		return &provider.Result{Content: "done"}, nil
	}}
	w := &progressWriter{}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "demo__echo"}}},
		Model:        "m",
		MaxToolIters: 5,
		ToolTrace:    agent.ToolTraceOff,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := channel.WithReplyWriter(context.Background(), w)
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if notes := w.traced(); len(notes) != 0 {
		t.Fatalf("notes = %v, want none when TOOL_TRACE=off", notes)
	}
}

func TestAgent_ToolProgress_MarksFailure(t *testing.T) {
	n := 0
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		n++
		if n == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "garmin__activities_list", Arguments: `{}`},
			}}, nil
		}
		return &provider.Result{Content: "garmin is down"}, nil
	}}
	w := &progressWriter{}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "garmin__activities_list"}}, err: errors.New("boom")},
		Model:        "m",
		MaxToolIters: 5,
		ToolTrace:    agent.ToolTraceFull,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := channel.WithReplyWriter(context.Background(), w)
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "ride?"}); err != nil {
		t.Fatal(err)
	}
	notes := w.traced()
	if len(notes) != 2 || !strings.Contains(notes[1], "failed") {
		t.Fatalf("notes = %v, want failure marker", notes)
	}
}

// Writers without UpdateProgress (stdio/slack/discord) must still work.
func TestAgent_ToolProgress_PlainWriterUnaffected(t *testing.T) {
	n := 0
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		n++
		if n == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "demo__echo", Arguments: `{}`},
			}}, nil
		}
		return &provider.Result{Content: "done"}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "demo__echo"}}},
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := channel.WithReplyWriter(context.Background(), &memWriter{})
	got, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "done" {
		t.Fatalf("reply = %q", got)
	}
}
