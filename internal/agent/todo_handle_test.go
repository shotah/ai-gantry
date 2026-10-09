package agent_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/session"
	"github.com/shotah/ai-gantry/internal/slash"
)

func TestTodoCommand_InCatalog(t *testing.T) {
	if !strings.Contains(slash.HelpText(), "/todo —") {
		t.Fatal(slash.HelpText())
	}
}

func newTodoAgent(t *testing.T) (*agent.Agent, *memory.Builtin) {
	t.Helper()
	sessions, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessions.Close() })
	mem, err := memory.OpenDB(sessions.DB())
	if err != nil {
		t.Fatal(err)
	}
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  sessions,
		Memory:    mem,
		Model:     "m",
		Location:  time.UTC,
		Now:       func() time.Time { return time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, mem
}

func TestAgent_TodoCommand(t *testing.T) {
	ctx := context.Background()
	a, mem := newTodoAgent(t)
	msg := channel.Message{SessionID: "s", UserID: "1", ChatID: "1", Text: "/todo"}

	got, err := a.Handle(ctx, msg)
	if err != nil || got != "nothing on the list" {
		t.Fatalf("empty %q %v", got, err)
	}

	dentist, err := mem.Store(ctx, memory.KindFact, "todo/dentist", "call to book a cleaning")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindFact, "todo/passport", "renew, Wed 11am"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindPreference, "pref/food", "tacos"); err != nil {
		t.Fatal(err)
	}
	got, err = a.Handle(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, fmt.Sprintf("#%d dentist: call to book a cleaning", dentist.ID)) || !strings.Contains(got, "passport: renew, Wed 11am") {
		t.Fatalf("list %q", got)
	}
	if strings.Contains(got, "tacos") || strings.Contains(got, "pocket list") {
		t.Fatalf("list leaked %q", got)
	}

	msg.Text = "/todo add call mom"
	if got, err = a.Handle(ctx, msg); err != nil || !strings.HasPrefix(got, "usage: /todo") {
		t.Fatalf("add is not a write %q %v", got, err)
	}
	if _, ok, _ := mem.ActiveByKindSubject(ctx, memory.KindFact, "todo/call-mom"); ok {
		t.Fatal("kernel must not slug and store")
	}

	msg.Text = fmt.Sprintf("/todo done %d", dentist.ID)
	if got, err = a.Handle(ctx, msg); err != nil || got != fmt.Sprintf("done: #%d dentist", dentist.ID) {
		t.Fatalf("done by id %q %v", got, err)
	}
	if _, ok, _ := mem.ActiveByKindSubject(ctx, memory.KindFact, "todo/dentist"); ok {
		t.Fatal("dentist still live")
	}

	msg.Text = fmt.Sprintf("/todo done %d", dentist.ID)
	if got, err = a.Handle(ctx, msg); err != nil || got != fmt.Sprintf("todo: #%d is gone — the list was updated", dentist.ID) {
		t.Fatalf("gone id %q %v", got, err)
	}

	msg.Text = "/todo done passport"
	if got, err = a.Handle(ctx, msg); err != nil || !strings.HasSuffix(got, " passport") || !strings.HasPrefix(got, "done: #") {
		t.Fatalf("done by slug %q %v", got, err)
	}
	msg.Text = "/todo done passport"
	if got, err = a.Handle(ctx, msg); err != nil || got != "todo: passport is gone — the list was updated" {
		t.Fatalf("gone slug %q %v", got, err)
	}

	// done by id only forgets a todo row: a food preference is not a task.
	food, _, _ := mem.ActiveByKindSubject(ctx, memory.KindPreference, "pref/food")
	msg.Text = fmt.Sprintf("/todo done %d", food.ID)
	if got, err = a.Handle(ctx, msg); err != nil || !strings.Contains(got, "is gone") {
		t.Fatalf("non-todo id %q %v", got, err)
	}
	if _, ok, _ := mem.ActiveByKindSubject(ctx, memory.KindPreference, "pref/food"); !ok {
		t.Fatal("/todo done must not delete a non-todo row")
	}
}

func TestAgent_TodoPrio(t *testing.T) {
	ctx := context.Background()
	a, mem := newTodoAgent(t)
	msg := channel.Message{SessionID: "s", UserID: "1", ChatID: "1", Text: "/todo"}

	dentist, err := mem.Store(ctx, memory.KindFact, "todo/dentist", "call to book a cleaning")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindFact, "todo/amazon", "return the box"); err != nil {
		t.Fatal(err)
	}

	// prio by id: same subject, marker leads the words, new row id.
	msg.Text = fmt.Sprintf("/todo prio %d !!", dentist.ID)
	got, err := a.Handle(ctx, msg)
	if err != nil || !strings.HasPrefix(got, "#") || !strings.HasSuffix(got, " dentist: !! call to book a cleaning") {
		t.Fatalf("prio by id %q %v", got, err)
	}
	live, ok, _ := mem.ActiveByKindSubject(ctx, memory.KindFact, "todo/dentist")
	if !ok || live.ID == dentist.ID || live.Content != "!! call to book a cleaning" {
		t.Fatalf("live row %+v ok=%v", live, ok)
	}

	// The list now leads with it, marker visible.
	msg.Text = "/todo"
	if got, err = a.Handle(ctx, msg); err != nil || !strings.HasPrefix(got, fmt.Sprintf("#%d dentist: !! call to book a cleaning", live.ID)) {
		t.Fatalf("list order %q %v", got, err)
	}

	// prio by slug, lowering to high.
	msg.Text = "/todo prio dentist !"
	if got, err = a.Handle(ctx, msg); err != nil || !strings.HasSuffix(got, " dentist: ! call to book a cleaning") {
		t.Fatalf("prio by slug %q %v", got, err)
	}

	// No marker clears it; the words are untouched.
	msg.Text = "/todo prio dentist"
	if got, err = a.Handle(ctx, msg); err != nil || !strings.HasSuffix(got, " dentist: call to book a cleaning") {
		t.Fatalf("clear %q %v", got, err)
	}
	live, _, _ = mem.ActiveByKindSubject(ctx, memory.KindFact, "todo/dentist")
	if live.Content != "call to book a cleaning" {
		t.Fatalf("content %q", live.Content)
	}

	// Bad marker is usage; unknown row is the gone line; the old id is gone too.
	msg.Text = "/todo prio dentist high"
	if got, err = a.Handle(ctx, msg); err != nil || !strings.HasPrefix(got, "usage: /todo") {
		t.Fatalf("bad marker %q %v", got, err)
	}
	msg.Text = "/todo prio passport !"
	if got, err = a.Handle(ctx, msg); err != nil || got != "todo: passport is gone — the list was updated" {
		t.Fatalf("gone slug %q %v", got, err)
	}
	msg.Text = fmt.Sprintf("/todo prio %d !", dentist.ID)
	if got, err = a.Handle(ctx, msg); err != nil || got != fmt.Sprintf("todo: #%d is gone — the list was updated", dentist.ID) {
		t.Fatalf("superseded id %q %v", got, err)
	}

	// prio never touches a non-todo row.
	food, err := mem.Store(ctx, memory.KindPreference, "pref/food", "tacos")
	if err != nil {
		t.Fatal(err)
	}
	msg.Text = fmt.Sprintf("/todo prio %d !!", food.ID)
	if got, err = a.Handle(ctx, msg); err != nil || !strings.Contains(got, "is gone") {
		t.Fatalf("non-todo id %q %v", got, err)
	}
	if live, _, _ := mem.ActiveByKindSubject(ctx, memory.KindPreference, "pref/food"); live.Content != "tacos" {
		t.Fatalf("pref touched %+v", live)
	}
}

func TestAgent_TodoLongListFooter(t *testing.T) {
	ctx := context.Background()
	a, mem := newTodoAgent(t)
	for i := 0; i < 11; i++ {
		if _, err := mem.Store(ctx, memory.KindFact, fmt.Sprintf("todo/t%02d", i), "n"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := a.Handle(ctx, channel.Message{SessionID: "s", UserID: "1", ChatID: "1", Text: "/todo"})
	if err != nil || !strings.HasSuffix(got, "11 open — a pocket list; prune, or use a tracker") {
		t.Fatalf("footer %q %v", got, err)
	}
	if strings.Count(got, "\n") != 11 {
		t.Fatalf("every row listed, no stamp cap: %q", got)
	}
}

func TestAgent_TodoUnconfigured(t *testing.T) {
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  newMemHistory(),
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", UserID: "1", Text: "/todo"})
	if err != nil || got != "todo: not configured" {
		t.Fatalf("%q %v", got, err)
	}
}
