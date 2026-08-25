package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
)

type stubSpark struct {
	enabled  bool
	def      string
	session  string
	resolved string
	set      string
	ensured  int
}

func (s *stubSpark) ProactiveEnabled() bool { return s.enabled }

func (s *stubSpark) DefaultQty() string { return s.def }

func (s *stubSpark) Window() (int, int) { return 6, 21 }

func (s *stubSpark) SessionQty(context.Context, string) (string, error) {
	return s.session, nil
}

func (s *stubSpark) ResolvedQty(context.Context, string) (string, error) {
	return s.resolved, nil
}

func (s *stubSpark) SetQty(_ context.Context, _, qty string) error {
	s.set = qty
	switch qty {
	case "0":
		s.session, s.resolved = "0", "0"
	case "":
		s.session, s.resolved = "", s.def
	default:
		s.session, s.resolved = qty, qty
	}
	return nil
}

func (s *stubSpark) EnsureFor(context.Context, cron.Delivery) (cron.Job, bool, error) {
	s.ensured++
	if !s.enabled || s.resolved == "" || s.resolved == "0" {
		return cron.Job{}, false, nil
	}
	return cron.Job{ID: 1}, true, nil
}

func TestAgent_SparkCommand(t *testing.T) {
	sp := &stubSpark{enabled: true, def: "3-5", resolved: "3-5"}
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  newMemHistory(),
		Model:     "m",
		Spark:     sp,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	msg := channel.Message{SessionID: "telegram:1:1", UserID: "1", ChatID: "1", Text: "/spark"}
	got, err := a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "looking-after-you") || !strings.Contains(got, "3-5") {
		t.Fatalf("status: %q %v", got, err)
	}

	msg.Text = "/spark 4"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "4 / day") {
		t.Fatalf("set qty: %q %v", got, err)
	}
	if sp.set != "4" || sp.ensured < 1 {
		t.Fatalf("set=%q ensured=%d", sp.set, sp.ensured)
	}

	msg.Text = "/engagement off"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "spark off") {
		t.Fatalf("off: %q %v", got, err)
	}
	if sp.set != "0" {
		t.Fatalf("set after off: %q", sp.set)
	}

	msg.Text = "/help"
	got, err = a.Handle(ctx, msg)
	if err != nil || !strings.Contains(got, "/engagement") || !strings.Contains(got, "/spark") {
		t.Fatalf("help: %q %v", got, err)
	}
}
