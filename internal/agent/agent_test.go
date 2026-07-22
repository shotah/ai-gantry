package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
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

func TestAgent_Handle_PersonaAndHistory(t *testing.T) {
	fc := &fakeCompleter{reply: "hi back"}
	a, err := agent.New(agent.Options{
		Persona:   "you are tim",
		Completer: fc,
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
	// system + prior user/assistant + new user
	if len(fc.last) != 4 {
		t.Fatalf("messages = %d, want 4", len(fc.last))
	}
}

func TestAgent_NewAndStatus(t *testing.T) {
	fc := &fakeCompleter{reply: "ok"}
	a, err := agent.New(agent.Options{Completer: fc, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "x"})

	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/new"})
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
