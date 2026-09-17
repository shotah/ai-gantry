package pendant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

func reactChannel(t *testing.T, streaming bool) *Channel {
	t.Helper()
	ch, err := New(Config{
		MailboxURL:    "wss://x.workers.dev/ws/kit",
		Bearer:        "tok",
		AllowedUsers:  []string{"1182"},
		StreamReplies: streaming,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

// The agent's [react …] goes out as a `react` frame on the human's inbound
// frame id; a reaction-only reply sends no `reply` at all.
func TestDispatch_ReactTokenWritesReactFrame(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "streaming"}[streaming], func(t *testing.T) {
			ch := reactChannel(t, streaming)
			fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
			raw, _ := json.Marshal(inboundFrame{Text: "thanks!", UserID: "1182", ID: "h-41"})
			err := ch.dispatch(context.Background(), fc, raw, func(ctx context.Context, _ channel.Message) (string, error) {
				channel.ReactionSinkFrom(ctx).Set("👍")
				return "", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			out := recvReply(t, fc.writes)
			if out.Kind != "react" || out.ID != "h-41" || out.Text != "👍" || out.UserID != "1182" {
				t.Fatalf("react frame = %+v", out)
			}
			select {
			case raw := <-fc.writes:
				t.Fatalf("reaction-only reply must write nothing more, got %s", raw)
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}

// Inbound frame without an id (older app): no sink, so the kernel's text
// fallback applies and the handler sees nothing to react on.
func TestDispatch_NoFrameIDNoSink(t *testing.T) {
	ch := reactChannel(t, false)
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	raw, _ := json.Marshal(inboundFrame{Text: "thanks!", UserID: "1182"})
	err := ch.dispatch(context.Background(), fc, raw, func(ctx context.Context, _ channel.Message) (string, error) {
		if channel.ReactionSinkFrom(ctx) != nil {
			t.Fatal("no inbound id: no sink")
		}
		return "👍", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if out := recvReply(t, fc.writes); out.Kind != "reply" || out.Text != "👍" {
		t.Fatalf("%+v", out)
	}
}

// Reply frames carry a crane id and the channel remembers their text, so a
// later reaction on that id can say what it was on.
func TestDispatch_ReplyCarriesIDAndIsRemembered(t *testing.T) {
	ch := reactChannel(t, false)
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	raw, _ := json.Marshal(inboundFrame{Text: "hi", UserID: "1182", ID: "h-1"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		return "Set a 6pm wake.", nil
	}); err != nil {
		t.Fatal(err)
	}
	out := recvReply(t, fc.writes)
	if out.Kind != "reply" || !strings.HasPrefix(out.ID, "r") {
		t.Fatalf("reply must carry an id: %+v", out)
	}
	if text, ok := ch.recent.Lookup(out.ID); !ok || text != "Set a 6pm wake." {
		t.Fatalf("recent[%s] = %q %v", out.ID, text, ok)
	}
}

// Push frames are remembered too — 👍 on a morning cron is the common case.
func TestPush_IsRemembered(t *testing.T) {
	ch := reactChannel(t, false)
	good := &fakeConn{writes: make(chan []byte, 4)}
	ch.dial = func(context.Context, string, http.Header) (conn, error) { return good, nil }
	if err := ch.Push(context.Background(), channel.Outbound{ID: "cron-9", Text: "Gym at 6?"}); err != nil {
		t.Fatal(err)
	}
	if text, ok := ch.recent.Lookup("cron-9"); !ok || text != "Gym at 6?" {
		t.Fatalf("recent = %q %v", text, ok)
	}
}

// A human's `react` frame settles (latest set wins), then runs one
// [reaction] turn naming our frame — with no sink, since there is no human
// message to react back on. An empty set cancels.
func TestDispatch_HumanReactSettlesIntoReactionTurn(t *testing.T) {
	prev := reactionSettle
	reactionSettle = 30 * time.Millisecond
	t.Cleanup(func() { reactionSettle = prev })

	ch := reactChannel(t, false)
	ch.recent.Remember("r-7", "Set a 6pm wake.")
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	got := make(chan channel.Message, 4)
	var handle channel.Handler = func(ctx context.Context, msg channel.Message) (string, error) {
		if channel.ReactionSinkFrom(ctx) != nil {
			t.Error("reaction turn must not offer a sink")
		}
		got <- msg
		return "", nil
	}
	send := func(id, text string) {
		raw, _ := json.Marshal(inboundFrame{Kind: "react", UserID: "1182", ID: id, Text: text})
		if err := ch.dispatch(context.Background(), fc, raw, handle); err != nil {
			t.Fatal(err)
		}
	}
	send("r-7", "❤️")
	send("r-7", "👍")
	select {
	case msg := <-got:
		if msg.Text != "[reaction] 👍 on: Set a 6pm wake." || msg.UserID != "1182" || msg.SessionID != channel.AgentSession {
			t.Fatalf("turn = %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("settled reaction never reached the handler")
	}

	send("r-7", "🔥")
	send("r-7", "")
	select {
	case msg := <-got:
		t.Fatalf("cleared reaction must not fire, got %+v", msg)
	case <-time.After(4 * reactionSettle):
	}

	// Unknown id: the target is named as unknown, still a turn.
	send("r-unknown", "👍")
	select {
	case msg := <-got:
		if msg.Text != "[reaction] 👍 on: (unknown message)" {
			t.Fatalf("turn = %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("unknown-id reaction never reached the handler")
	}
}

// The react frame rides the reply path: a dead dispatch socket falls back
// to a fresh dial rather than losing the reaction.
func TestDispatch_ReactFallsBackToDial(t *testing.T) {
	ch := reactChannel(t, false)
	dead := &fakeConn{reads: make(chan []byte, 1), writeErr: io.ErrClosedPipe}
	good := &fakeConn{writes: make(chan []byte, 4)}
	ch.dial = func(context.Context, string, http.Header) (conn, error) { return good, nil }
	raw, _ := json.Marshal(inboundFrame{Text: "thanks!", UserID: "1182", ID: "h-41"})
	err := ch.dispatch(context.Background(), dead, raw, func(ctx context.Context, _ channel.Message) (string, error) {
		channel.ReactionSinkFrom(ctx).Set("👍")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if out := recvReply(t, good.writes); out.Kind != "react" || out.ID != "h-41" || out.Text != "👍" {
		t.Fatalf("dialed react = %+v", out)
	}
}
