package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestAgent_WaitTokenArmsAndUserClears(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	jobs, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	var lastPrompt string
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
			for i := len(req.Messages) - 1; i >= 0; i-- {
				if req.Messages[i].Role == provider.RoleSystem && strings.Contains(req.Messages[i].Content, "[conversation]") {
					lastPrompt = req.Messages[i].Content
					break
				}
			}
			lastUser := ""
			for i := len(req.Messages) - 1; i >= 0; i-- {
				if req.Messages[i].Role == provider.RoleUser {
					lastUser = req.Messages[i].Content
					break
				}
			}
			if strings.Contains(lastUser, "pizza") {
				return &provider.Result{Content: "great, pizza it is"}, nil
			}
			return &provider.Result{Content: "Thai or pizza?\n[wait]"}, nil
		}},
		Sessions: sess,
		Wait:     &cron.WaitService{State: sess, Jobs: jobs, TZ: "UTC"},
		Model:    "m",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := a.Handle(ctx, channel.Message{SessionID: "s", UserID: "u", ChatID: "1", Text: "dinner?"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "[wait]") {
		t.Fatalf("pushed text leaked [wait]: %q", got)
	}
	if !strings.Contains(got, "Thai or pizza?") {
		t.Fatalf("got=%q", got)
	}
	st, err := sess.TalkState(ctx, "s")
	if err != nil || !st.WaitingForReply {
		t.Fatalf("armed: %+v err=%v", st, err)
	}
	hist, err := sess.Messages(ctx, "s")
	if err != nil || len(hist) < 2 || !strings.Contains(hist[len(hist)-1].Content, "[wait]") {
		t.Fatalf("history should keep [wait]: %+v err=%v", hist, err)
	}
	if n := countKindAgent(t, jobs, cron.KindFollowUp); n != 1 {
		t.Fatalf("followup jobs=%d", n)
	}

	got, err = a.Handle(ctx, channel.Message{SessionID: "s", UserID: "u", ChatID: "1", Text: "pizza"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "pizza it is") {
		t.Fatalf("reply=%q", got)
	}
	st, err = sess.TalkState(ctx, "s")
	if err != nil || st.WaitingForReply {
		t.Fatalf("user reply must clear wait: %+v err=%v", st, err)
	}
	if n := countKindAgent(t, jobs, cron.KindFollowUp); n != 0 {
		t.Fatalf("followups after user=%d", n)
	}
	if !strings.Contains(lastPrompt, "waiting_for_reply=false") {
		t.Fatalf("second turn footer after clear: %q", lastPrompt)
	}
}

func TestAgent_ConversationFooterOnPrompt(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	if err := sess.ArmWait(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	var saw string
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
			for _, m := range req.Messages {
				if strings.Contains(m.Content, "[conversation]") {
					saw = m.Content
				}
			}
			return &provider.Result{Content: "ok"}, nil
		}},
		Sessions: sess,
		Model:    "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "[cron] Scheduled job — ping\n\nbody"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saw, "waiting_for_reply=true") || !strings.Contains(saw, "last_speaker=agent") {
		t.Fatalf("footer=%q", saw)
	}
}

func countKindAgent(t *testing.T, store *cron.Store, kind string) int {
	t.Helper()
	jobs, err := store.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, j := range jobs {
		if j.Kind == kind {
			n++
		}
	}
	return n
}
