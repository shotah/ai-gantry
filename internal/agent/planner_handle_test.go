package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
)

type stubPlanner struct {
	enabled  bool
	def      string
	session  string
	resolved string
	set      string
	ensured  int
}

func (s *stubPlanner) ProactiveEnabled() bool { return s.enabled }

func (s *stubPlanner) DefaultAt() string { return s.def }

func (s *stubPlanner) SessionAt(context.Context, string) (string, error) {
	return s.session, nil
}

func (s *stubPlanner) ResolvedAt(context.Context, string) (string, error) {
	return s.resolved, nil
}

func (s *stubPlanner) SetAt(_ context.Context, _, at string) error {
	s.set = at
	switch at {
	case "0":
		s.session, s.resolved = "0", "0"
	case "":
		s.session, s.resolved = "", s.def
	default:
		s.session, s.resolved = at, at
	}
	return nil
}

func (s *stubPlanner) EnsureFor(context.Context, cron.Delivery) (cron.Job, bool, error) {
	s.ensured++
	if !s.enabled || s.resolved == "" || s.resolved == "0" {
		return cron.Job{}, false, nil
	}
	return cron.Job{ID: 1, Expr: s.resolved}, true, nil
}

func TestAgent_PlannerCommand(t *testing.T) {
	sp := &stubPlanner{enabled: true, def: "07:10", resolved: "07:10"}
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  newMemHistory(),
		Model:     "m",
		Planner:   sp,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	msg := channel.Message{SessionID: "s", UserID: "1", ChatID: "1", Text: "/planner"}
	got, err := a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "daily planner") || !strings.Contains(got, "07:10") {
		t.Fatalf("status: %q %v", got, err)
	}

	msg.Text = "/planner 09:30"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "09:30") {
		t.Fatalf("set: %q %v", got, err)
	}
	if sp.set != "09:30" || sp.ensured < 1 {
		t.Fatalf("set=%q ensured=%d", sp.set, sp.ensured)
	}

	msg.Text = "/planner off"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "planner off") {
		t.Fatalf("off: %q %v", got, err)
	}
	if sp.set != "0" {
		t.Fatalf("set after off: %q", sp.set)
	}

	msg.Text = "/help"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "/planner") || strings.Contains(got, "/spark") || strings.Contains(got, "/engagement") {
		t.Fatalf("help: %q %v", got, err)
	}
}
