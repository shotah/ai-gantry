package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/aims"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/session"
	"github.com/shotah/ai-gantry/internal/slash"
)

func TestParseAimsCommand(t *testing.T) {
	// parseAimsCommand is unexported; the handle tests cover the forms.
	if !strings.Contains(slash.HelpText(), "/aims —") {
		t.Fatal(slash.HelpText())
	}
}

func TestAgent_AimsCommand(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, loc)
	sessions, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessions.Close() })
	mem, err := memory.OpenDB(sessions.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindInsight, "aim/training", "3x gym"); err != nil {
		t.Fatal(err)
	}
	store, err := aims.OpenDB(sessions.DB(), loc, mem)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Log(ctx, aims.Event{What: "gym", Day: "2026-09-27", Note: "praised"}, map[string]int{"training": 2}); err != nil {
		t.Fatal(err)
	}
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  sessions,
		Memory:    mem,
		Model:     "m",
		Location:  loc,
		Now:       func() time.Time { return now },
		Aims:      store,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := channel.Message{SessionID: "s", UserID: "1", ChatID: "1", Text: "/aims"}
	got, err := a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "training —") || !strings.Contains(got, "streak 1") {
		t.Fatalf("board %q %v", got, err)
	}
	msg.Text = "/aims training"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "#") || !strings.Contains(got, "gym") || !strings.Contains(got, "training +2") {
		t.Fatalf("area %q %v", got, err)
	}
	msg.Text = "/aims rubric"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "clean day") || !strings.Contains(got, "+3") {
		t.Fatalf("rubric %q %v", got, err)
	}
	msg.Text = "/aims block training 2026-09-01 2026-12-01"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "2026-09-01") {
		t.Fatalf("block %q %v", got, err)
	}
	msg.Text = "/aims nosuch"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "unknown aim") {
		t.Fatalf("unknown %q %v", got, err)
	}
}

func TestAgent_AimsUnconfigured(t *testing.T) {
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  newMemHistory(),
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", UserID: "1", Text: "/aims"})
	if err != nil || !strings.Contains(got, "not configured") {
		t.Fatalf("%q %v", got, err)
	}
}
