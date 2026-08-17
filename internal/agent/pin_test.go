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
			if strings.Contains(m.Content, "[current time]") {
				clock = m.Content
			}
		}
		if !strings.Contains(clock, "[last pin]") || !strings.Contains(clock, "37.386051") || !strings.Contains(clock, "Cafe") {
			t.Errorf("clock missing last pin: %q", clock)
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
