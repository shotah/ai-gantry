package agent_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

type fakeSelfNotes struct {
	mu       sync.Mutex
	content  string
	wrote    []string
	writeErr error
}

func (f *fakeSelfNotes) Read() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.content, nil
}

func (f *fakeSelfNotes) Write(content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeErr != nil {
		return f.writeErr
	}
	f.wrote = append(f.wrote, content)
	f.content = content
	return nil
}

func seedHistory(t *testing.T, h *memHistory, sessionID string, turns int) {
	t.Helper()
	for i := 0; i < turns; i++ {
		if err := h.Append(context.Background(), sessionID,
			session.Message{Role: session.RoleUser, Content: "guess my number"},
			session.Message{Role: session.RoleAssistant, Content: "is it 42, Boss?"},
		); err != nil {
			t.Fatal(err)
		}
	}
}

// /new with enough history distills personality into SELF.md, then resets.
func TestAgent_NewDistillsSelfNotes(t *testing.T) {
	var reqs []provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		reqs = append(reqs, req)
		// Fenced reply exercises the code-fence unwrap too.
		return &provider.Result{Content: "```markdown\n# SELF.md — Who You Are Becoming\n- calls the human Boss\n```"}, nil
	}}
	notes := &fakeSelfNotes{content: "# SELF.md — Who You Are Becoming\n- dry humor"}
	hist := newMemHistory()
	hist.setSummary("s", "Facts: chris likes espresso\nVoice: gag: \"that gull had a mortgage\"")
	seedHistory(t, hist, "s", 3) // 6 messages = distill threshold
	a, err := agent.New(agent.Options{
		Completer: fc,
		Sessions:  hist,
		SelfNotes: notes,
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/new"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "personality distilled") {
		t.Fatalf("reply = %q", reply)
	}
	if len(reqs) != 1 {
		t.Fatalf("model calls = %d, want 1 distill call", len(reqs))
	}
	var joined strings.Builder
	for _, m := range reqs[0].Messages {
		joined.WriteString(m.Content + "\n")
	}
	for _, want := range []string{"[current SELF.md]", "- dry humor", "[transcript]", "guess my number", "[session voice]", "gull"} {
		if !strings.Contains(joined.String(), want) {
			t.Fatalf("distill prompt missing %q in %q", want, joined.String())
		}
	}
	if len(notes.wrote) != 1 || notes.wrote[0] != "# SELF.md — Who You Are Becoming\n- calls the human Boss" {
		t.Fatalf("wrote = %q", notes.wrote)
	}
	if msgs, _ := hist.Messages(context.Background(), "s"); len(msgs) != 0 {
		t.Fatalf("history not reset: %d messages", len(msgs))
	}
}

// Short sessions have no personality worth keeping — no distill call.
func TestAgent_NewShortSessionSkipsDistill(t *testing.T) {
	fc := &fakeCompleter{}
	notes := &fakeSelfNotes{}
	hist := newMemHistory()
	seedHistory(t, hist, "s", 2) // 4 messages < threshold
	a, err := agent.New(agent.Options{
		Completer: fc,
		Sessions:  hist,
		SelfNotes: notes,
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/new"})
	if err != nil || reply != "session reset" {
		t.Fatalf("reply = %q, err = %v", reply, err)
	}
	if fc.calls != 0 {
		t.Fatalf("model calls = %d, want 0", fc.calls)
	}
	if len(notes.wrote) != 0 {
		t.Fatalf("wrote = %q, want none", notes.wrote)
	}
}

// A dead provider must never block /new — reset proceeds without the distill.
func TestAgent_NewDistillFailureStillResets(t *testing.T) {
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return nil, errors.New("llm down")
	}}
	hist := newMemHistory()
	seedHistory(t, hist, "s", 4)
	a, err := agent.New(agent.Options{
		Completer: fc,
		Sessions:  hist,
		SelfNotes: &fakeSelfNotes{},
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/new"})
	if err != nil || reply != "session reset" {
		t.Fatalf("reply = %q, err = %v", reply, err)
	}
	if msgs, _ := hist.Messages(context.Background(), "s"); len(msgs) != 0 {
		t.Fatalf("history not reset: %d messages", len(msgs))
	}
}

func TestAgent_NewParksFactsInMemory(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	hist := newMemHistory()
	hist.setSummary("chat", "Facts: chris likes espresso\nVoice: gag: \"that gull had a mortgage\"")
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  hist,
		Memory:    store,
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "chat", Text: "/new"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "session facts parked in memory") {
		t.Fatalf("reply = %q", reply)
	}
	hits, err := store.Recall(context.Background(), "espresso", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range hits {
		if e.Kind == memory.KindEpisode && e.Subject == "session" && strings.Contains(e.Content, "espresso") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("parked episode not recalled: %+v", hits)
	}
}

func TestAgent_NewDistillsFromVoiceWithoutLongHistory(t *testing.T) {
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Content: "# SELF.md — Who You Are Becoming\n- running gag: that gull had a mortgage"}, nil
	}}
	notes := &fakeSelfNotes{content: "# SELF.md — Who You Are Becoming\n- dry humor"}
	hist := newMemHistory()
	hist.setSummary("v", "Facts: leftover\nVoice: gag: \"that gull had a mortgage\"")
	seedHistory(t, hist, "v", 1) // 2 messages < distill threshold; voice still counts
	a, err := agent.New(agent.Options{
		Completer: fc,
		Sessions:  hist,
		SelfNotes: notes,
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "v", Text: "/new"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "personality distilled") {
		t.Fatalf("reply = %q", reply)
	}
	if len(notes.wrote) != 1 {
		t.Fatalf("wrote = %q", notes.wrote)
	}
}
