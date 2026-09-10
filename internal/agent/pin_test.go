package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/here"
	"github.com/shotah/ai-gantry/internal/provider"
)

func TestHandle_LastPinInClockFooter(t *testing.T) {
	here.Set("pin-s", here.Pin{
		Lat: 37.386051, Lon: -122.083855, Label: "Cafe",
		At: time.Now().Add(-2 * time.Minute),
	})
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		var clock string
		for _, m := range req.Messages {
			if m.Role == provider.RoleUser && strings.Contains(m.Content, "[current time]") {
				clock = m.Content
			}
		}
		if !strings.Contains(clock, "[last pin]") || !strings.Contains(clock, "37.386051") || !strings.Contains(clock, "Cafe") {
			t.Errorf("user turn missing last pin: %q", clock)
		}
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: "pin-s", Text: "restaurants"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandle_PendantGPSLeadsClockFooterOnUserTurn(t *testing.T) {
	sid := "pendant:kit:1182"
	here.Set(sid, here.Pin{
		Lat: 47.6, Lon: -122.3,
		At: time.Now(),
	})
	hist := newMemHistory()
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		var user string
		for _, m := range req.Messages {
			if m.Role == provider.RoleUser {
				user = m.Content
			}
		}
		ask, clock, ok := strings.Cut(user, "\n\n")
		if !ok {
			t.Fatalf("user turn missing clock footer: %q", user)
		}
		if strings.Contains(ask, "[last pin]") || strings.Contains(ask, "47.6") {
			t.Errorf("pendant GPS must not be mixed into their words: %q", ask)
		}
		if !strings.Contains(clock, "[last pin]") || !strings.Contains(clock, "47.600000") || !strings.Contains(clock, "just now") {
			t.Errorf("clock missing this-send pin: %q", clock)
		}
		if !strings.Contains(clock, "[current time]") {
			t.Errorf("clock missing current time: %q", clock)
		}
		pinAt := strings.Index(clock, "[last pin]")
		timeAt := strings.Index(clock, "[current time]")
		if pinAt < 0 || timeAt < 0 || pinAt > timeAt {
			t.Errorf("last pin must lead the clock footer: %q", clock)
		}
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: hist, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: sid, Text: "what's near me"}); err != nil {
		t.Fatal(err)
	}
	stored, err := hist.Messages(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range stored {
		if strings.Contains(m.Content, "[last pin]") || strings.Contains(m.Content, "[current time]") || strings.Contains(m.Content, "47.600000") {
			t.Fatalf("clock/pin leaked into history: %+v", m)
		}
	}
}
