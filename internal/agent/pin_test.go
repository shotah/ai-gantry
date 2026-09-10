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

func TestHandle_LocationOnThisSend(t *testing.T) {
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		var clock string
		for _, m := range req.Messages {
			if m.Role == provider.RoleUser && strings.Contains(m.Content, "[current time]") {
				clock = m.Content
			}
		}
		if !strings.Contains(clock, "[location]") || !strings.Contains(clock, "37.386051") || !strings.Contains(clock, "Cafe") {
			t.Errorf("user turn missing location: %q", clock)
		}
		if !strings.Contains(clock, "just now") {
			t.Errorf("this-send GPS should be just now: %q", clock)
		}
		if strings.Contains(clock, "[last pin]") {
			t.Errorf("must not say last pin: %q", clock)
		}
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{
		SessionID: "loc-this-send",
		Text:      "restaurants",
		Geo:       &channel.Geo{Lat: 37.386051, Lon: -122.083855, Label: "Cafe"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHandle_NoLocationWithoutGeo(t *testing.T) {
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "[location]") || strings.Contains(m.Content, "[last pin]") {
				t.Errorf("no geo on this send: %q", m.Content)
			}
		}
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: "loc-empty", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandle_CachedLocationOnLaterTurn(t *testing.T) {
	sid := "loc-cached"
	here.Set(sid, here.Pin{
		Lat: 47.6, Lon: -122.3,
		At: time.Now().Add(-3 * time.Minute),
	})
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		var user string
		for _, m := range req.Messages {
			if m.Role == provider.RoleUser {
				user = m.Content
			}
		}
		if !strings.Contains(user, "hi") || !strings.Contains(user, "[location]") || !strings.Contains(user, "47.600000") {
			t.Errorf("cached GPS missing from user lines: %q", user)
		}
		if !strings.Contains(user, "3m ago") {
			t.Errorf("cached GPS should show age: %q", user)
		}
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: newMemHistory(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: sid, Text: "hi"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandle_PendantGPSLeadsClockFooterOnUserTurn(t *testing.T) {
	sid := "pendant:kit:loc-footer"
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
		if strings.Contains(ask, "[location]") || strings.Contains(ask, "47.6") {
			t.Errorf("GPS must not be mixed into their words: %q", ask)
		}
		if !strings.Contains(clock, "[location ±8m]") || !strings.Contains(clock, "47.600000") {
			t.Errorf("clock missing this-send GPS: %q", clock)
		}
		if !strings.Contains(clock, "[current time]") {
			t.Errorf("clock missing current time: %q", clock)
		}
		locAt := strings.Index(clock, "[location")
		timeAt := strings.Index(clock, "[current time]")
		if locAt < 0 || timeAt < 0 || locAt > timeAt {
			t.Errorf("location must lead the clock footer: %q", clock)
		}
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: hist, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{
		SessionID: sid,
		Text:      "what's near me",
		Geo:       &channel.Geo{Lat: 47.6, Lon: -122.3, AccuracyM: 8},
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := hist.Messages(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range stored {
		if strings.Contains(m.Content, "[location]") || strings.Contains(m.Content, "[current time]") || strings.Contains(m.Content, "47.600000") {
			t.Fatalf("clock/GPS leaked into history: %+v", m)
		}
	}
}
