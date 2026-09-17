package agent_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

func reactAgent(t *testing.T, reply string) (*agent.Agent, *memHistory, *fakeCompleter, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	hist := newMemHistory()
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Content: reply}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer: fc,
		Sessions:  hist,
		Logger:    slog.New(slog.NewJSONHandler(&buf, nil)),
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, hist, fc, &buf
}

// A mouth that can react gets the emoji on its sink; the text stays as
// written (empty here — the reaction is the reply) and history keeps the
// token so the model later sees it reacted.
func TestAgent_ReactToken_SinkGetsEmoji(t *testing.T) {
	a, hist, _, _ := reactAgent(t, "[react 👍]")
	ctx, sink := channel.AttachReactionSink(context.Background())
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", UserID: "7", Text: "thanks, sounds good"})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "" || sink.Emoji() != "👍" {
		t.Fatalf("reply=%q sink=%q", reply, sink.Emoji())
	}
	msgs, _ := hist.Messages(ctx, "s")
	if last := msgs[len(msgs)-1]; last.Role != session.RoleAssistant || last.Content != "[react 👍]" {
		t.Fatalf("history kept %+v, want the token", last)
	}
}

// React and write: the sink takes the emoji, the line goes out as text.
func TestAgent_ReactToken_WithText(t *testing.T) {
	a, _, _, _ := reactAgent(t, "Set a 6pm wake to ask how it went.\n[react 👍]")
	ctx, sink := channel.AttachReactionSink(context.Background())
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", UserID: "7", Text: "heading to the gym"})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "Set a 6pm wake to ask how it went." || sink.Emoji() != "👍" {
		t.Fatalf("reply=%q sink=%q", reply, sink.Emoji())
	}
}

// No sink (stdio, Discord, Slack), a spoken turn, or an emoji off the
// palette: the emoji rides as text when there is none — never silence — and
// is dropped when there is text.
func TestAgent_ReactToken_FallsBackToText(t *testing.T) {
	cases := []struct {
		name, model, input, want string
		sink                     bool
		wantSink                 string
	}{
		{name: "no sink, react only", model: "[react 👍]", want: "👍"},
		{name: "no sink, with text", model: "Noted.\n[react 👍]", want: "Noted."},
		{name: "spoken turn", model: "[react 👍]", input: "spoken", sink: true, want: "👍"},
		{name: "off palette", model: "[react 💩]", sink: true, want: "💩"},
		{name: "negative on palette", model: "[react 👎]", sink: true, want: "", wantSink: "👎"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _, _ := reactAgent(t, tc.model)
			ctx := context.Background()
			var sink *channel.ReactionSink
			if tc.sink {
				ctx, sink = channel.AttachReactionSink(ctx)
			}
			reply, err := a.Handle(ctx, channel.Message{SessionID: "s", UserID: "7", Text: "ok", Input: tc.input})
			if err != nil {
				t.Fatal(err)
			}
			if reply != tc.want || sink.Emoji() != tc.wantSink {
				t.Fatalf("reply=%q sink=%q; want %q / %q", reply, sink.Emoji(), tc.want, tc.wantSink)
			}
		})
	}
}

// Their 👍 with nothing pending is recorded as the pair the model would have
// written, with no model call and a zero-cost turn perf line.
func TestAgent_ReactionTriage_IdlePositiveSkipsModel(t *testing.T) {
	a, hist, fc, buf := reactAgent(t, "should not be asked")
	ctx := context.Background()
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", UserID: "7", Text: "[reaction] 👍 ❤️ on: Set a 6pm wake."})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "" || fc.calls != 0 {
		t.Fatalf("reply=%q model calls=%d, want silent and none", reply, fc.calls)
	}
	msgs, _ := hist.Messages(ctx, "s")
	if len(msgs) != 2 || msgs[0].Role != session.RoleUser || !strings.HasPrefix(msgs[0].Content, "[reaction] 👍 ❤️ on:") ||
		msgs[1].Role != session.RoleAssistant || msgs[1].Content != "[silent]" {
		t.Fatalf("history = %+v", msgs)
	}
	rec := lastTurnPerf(t, buf.String())
	if rec["source"] != "reaction" || rec["outcome"] != "silent" || rec["iterations"] != float64(0) || rec["user_id"] != "7" {
		t.Fatalf("perf = %v", rec)
	}
}

// Their 👍 on the question the agent is waiting on is the answer: the wait
// clears and the follow-up pokes are cancelled, like a text reply would.
func TestAgent_ReactionOnWaitingQuestionClearsWait(t *testing.T) {
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
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Content: "Thursday 7–8 blocked."}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer: fc,
		Sessions:  sess,
		Wait:      &cron.WaitService{State: sess, Jobs: jobs, TZ: "UTC"},
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The agent asked and armed the wait with its first follow-up poke.
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", UserID: "u", Text: "look at my week"}); err != nil {
		t.Fatal(err)
	}
	fc.fn = func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Content: "Block Thursday 7–8 for the gym?\n[wait]"}, nil
	}
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", UserID: "u", Text: "anything open?"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := sess.TalkState(ctx, "s"); !st.WaitingForReply {
		t.Fatal("precondition: agent should be waiting")
	}
	fc.fn = func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Content: "Thursday 7–8 blocked."}, nil
	}
	if _, err := a.Handle(ctx, channel.Message{SessionID: "s", UserID: "u", Text: "[reaction] 👍 on: Block Thursday 7–8 for the gym?"}); err != nil {
		t.Fatal(err)
	}
	if fc.calls != 3 {
		t.Fatalf("waiting 👍 must reach the model; calls=%d", fc.calls)
	}
	if st, _ := sess.TalkState(ctx, "s"); st.WaitingForReply {
		t.Fatal("their 👍 answered the question; wait must clear")
	}
	list, err := jobs.ListSession(ctx, "s", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range list {
		if strings.Contains(j.Prompt, cron.FollowUpTurnMarker) {
			t.Fatalf("follow-up poke survived their answer: %+v", j)
		}
	}
}

// Waiting on them, 👍 is an answer; a 👎 is never nothing. Both reach the model.
func TestAgent_ReactionTriage_WaitingOrNegativeReachesModel(t *testing.T) {
	cases := []struct {
		name, text string
		waiting    bool
	}{
		{name: "waiting, thumbs up", text: "[reaction] 👍 on: Gym at 6?", waiting: true},
		{name: "idle, thumbs down", text: "[reaction] 👎 on: Set a 6pm wake."},
		{name: "idle, question", text: "[reaction] 👍 ❓ on: Set a 6pm wake."},
		{name: "idle, custom emoji", text: "[reaction] [custom:123] on: Set a 6pm wake."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, hist, fc, _ := reactAgent(t, "Booked it.")
			hist.setWaiting("s", tc.waiting)
			reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", UserID: "7", Text: tc.text})
			if err != nil {
				t.Fatal(err)
			}
			if fc.calls != 1 || reply != "Booked it." {
				t.Fatalf("model calls=%d reply=%q", fc.calls, reply)
			}
		})
	}
}
