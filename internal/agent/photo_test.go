package agent_test

import (
	"context"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

func TestAgent_Handle_PhotoVision(t *testing.T) {
	fc := &capturingCompleter{}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Handle(context.Background(), channel.Message{
		SessionID: "s",
		Images:    []channel.Image{{URL: "data:image/jpeg;base64,qq"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply == "" {
		t.Fatal("empty reply")
	}
	if len(fc.last.Messages) == 0 {
		t.Fatal("no messages")
	}
	var photo *provider.Message
	for i := range fc.last.Messages {
		m := &fc.last.Messages[i]
		if m.Role == provider.RoleUser && m.Content == "[photo]" {
			photo = m
			break
		}
	}
	if photo == nil || len(photo.ImageURLs) != 1 {
		t.Fatalf("missing photo user message: %+v", fc.last.Messages)
	}
}

type capturingCompleter struct {
	last provider.Request
}

func (c *capturingCompleter) Complete(_ context.Context, req provider.Request) (*provider.Result, error) {
	c.last = req
	return &provider.Result{Content: "saw it"}, nil
}
