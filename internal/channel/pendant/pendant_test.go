package pendant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/aims"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/here"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestNew_RequiresURLBearerAllowlist(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("url")
	}
	if _, err := New(Config{MailboxURL: "wss://x.workers.dev/ws/kit"}); err == nil {
		t.Fatal("bearer")
	}
	if _, err := New(Config{MailboxURL: "wss://x.workers.dev/ws/kit", Bearer: "tok"}); err == nil {
		t.Fatal("allowlist")
	}
	if _, err := New(Config{MailboxURL: "not a url", Bearer: "tok", AllowedUsers: []string{"s"}}); err == nil {
		t.Fatal("bad url")
	}
	ch, err := New(Config{
		MailboxURL:   "https://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{" 1182:ada@example.com ", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ch.isAllowed("1182", "") || ch.isAllowed("1182:ada@example.com", "") || ch.slug != "kit" {
		t.Fatalf("slug=%q allowed=%v", ch.slug, ch.allowed)
	}
	if !ch.isAllowed("", "ada@example.com") {
		t.Fatal("email alias")
	}
	if _, err := New(Config{MailboxURL: "wss://x.workers.dev/ws/kit", Bearer: "tok", AllowedUsers: []string{"ada@example.com"}}); err != nil {
		t.Fatalf("email-only boot: %v", err)
	}
	if _, err := New(Config{MailboxURL: "wss://x.workers.dev/ws/kit", Bearer: "tok", AllowedUsers: []string{"nope"}}); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("junk: %v", err)
	}
	if !strings.HasPrefix(ch.mailbox, "wss://") {
		t.Fatalf("mailbox = %q", ch.mailbox)
	}
	if MailboxSlug("wss://x.workers.dev/ws/kit") != "kit" {
		t.Fatal(MailboxSlug("wss://x.workers.dev/ws/kit"))
	}
	if MailboxSlug("bad") != "crane" {
		t.Fatal(MailboxSlug("bad"))
	}
}

type fakeConn struct {
	reads    chan []byte
	writes   chan []byte
	once     sync.Once
	writeErr error
}

func (f *fakeConn) ReadMessage() (int, []byte, error) {
	b, ok := <-f.reads
	if !ok {
		return 0, nil, io.EOF
	}
	return 1, b, nil
}

func (f *fakeConn) WriteMessage(_ int, data []byte) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.writes <- append([]byte(nil), data...)
	return nil
}

func (f *fakeConn) Close() error {
	f.once.Do(func() {
		if f.reads != nil {
			close(f.reads)
		}
	})
	return nil
}

func recvOutbound(t *testing.T, writes <-chan []byte) outboundFrame {
	t.Helper()
	select {
	case raw := <-writes:
		var out outboundFrame
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame")
	}
	return outboundFrame{}
}

func recvReply(t *testing.T, writes <-chan []byte) outboundFrame {
	t.Helper()
	for {
		out := recvOutbound(t, writes)
		if out.Kind == "typing" {
			continue
		}
		return out
	}
}

func TestDispatch_GeoOnMessageAndReply(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	raw, _ := json.Marshal(inboundFrame{
		Text:   "near me",
		UserID: "1182",
		Context: &frameContext{
			At:  time.Date(2026, 9, 4, 18, 0, 0, 0, time.UTC).Format(time.RFC3339),
			Geo: &geo{Lat: 47.6, Lon: -122.3},
		},
	})
	if err := ch.dispatch(context.Background(), fc, raw, func(_ context.Context, msg channel.Message) (string, error) {
		if msg.UserID != "1182" {
			t.Fatalf("userid %q", msg.UserID)
		}
		if msg.SessionID != channel.AgentSession {
			t.Fatalf("sid %q", msg.SessionID)
		}
		if msg.Text != "near me" {
			t.Fatalf("text %q", msg.Text)
		}
		if strings.Contains(msg.Text, "[location]") {
			t.Fatal("must not stuff [location] into Text")
		}
		if msg.Geo == nil || msg.Geo.Lat != 47.6 || msg.Geo.Lon != -122.3 {
			t.Fatalf("geo %+v", msg.Geo)
		}
		p, ok := here.Get(msg.SessionID)
		if !ok || p.Lat != 47.6 {
			t.Fatalf("cache %+v ok=%v", p, ok)
		}
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}
	out := recvReply(t, fc.writes)
	if out.Kind != "reply" || out.Text != "ok" || out.UserID != "1182" {
		t.Fatalf("%+v", out)
	}
}

func TestDispatch_BareGeoSilentAndDeny(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 1)}
	called := false
	raw, _ := json.Marshal(inboundFrame{
		UserID:  "1182",
		Context: &frameContext{Geo: &geo{Lat: 1, Lon: 2}},
	})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		called = true
		return "nope", nil
	}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("bare geo must not start a turn")
	}
	p, ok := here.Get(channel.AgentSession)
	if !ok || p.Lat != 1 || p.Lon != 2 {
		t.Fatalf("silent geo should cache last known: %+v ok=%v", p, ok)
	}
	select {
	case <-fc.writes:
		t.Fatal("no reply on silent pin")
	default:
	}

	raw, _ = json.Marshal(inboundFrame{Text: "hi", UserID: "999"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		t.Fatal("denied user")
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPush_AllowlistAndLive(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte), writes: make(chan []byte, 1)}
	ch.setLive(fc)
	if err := ch.Push(context.Background(), channel.Outbound{UserID: "nope", ChatID: "1", Text: "ping"}); err != nil {
		t.Fatal(err)
	}
	raw := <-fc.writes
	var out outboundFrame
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "push" || out.Text != "ping" || out.UserID != "1182" {
		t.Fatalf("%+v", out)
	}
}

// Nothing to say means no frame — not a push with blank text.
func TestPush_EmptyTextWritesNothing(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte), writes: make(chan []byte, 1)}
	ch.setLive(fc)
	if err := ch.Push(context.Background(), channel.Outbound{Text: "  \n"}); err != nil {
		t.Fatal(err)
	}
	select {
	case raw := <-fc.writes:
		t.Fatalf("empty push reached the wire: %s", raw)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPush_BroadcastWhenNoSub(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"ada@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte), writes: make(chan []byte, 1)}
	ch.setLive(fc)
	if err := ch.Push(context.Background(), channel.Outbound{Text: "all"}); err != nil {
		t.Fatal(err)
	}
	raw := <-fc.writes
	if strings.Contains(string(raw), "user_id") {
		t.Fatalf("broadcast must omit user_id: %s", raw)
	}
	var out outboundFrame
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "push" || out.Text != "all" || out.UserID != "" {
		t.Fatalf("%+v", out)
	}
}

func TestPush_DialsWhenIdle(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte), writes: make(chan []byte, 1)}
	ch.dial = func(_ context.Context, _ string, h http.Header) (conn, error) {
		if h.Get("Authorization") != "Bearer tok" {
			t.Fatalf("auth %q", h.Get("Authorization"))
		}
		return fc, nil
	}
	if err := ch.Push(context.Background(), channel.Outbound{Text: "cron"}); err != nil {
		t.Fatal(err)
	}
	raw := <-fc.writes
	var out outboundFrame
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "push" || out.UserID != "1182" {
		t.Fatalf("%+v", out)
	}
}

func TestPush_IgnoresStoredDest(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{writes: make(chan []byte, 1)}
	ch.setLive(fc)
	if err := ch.Push(context.Background(), channel.Outbound{
		SessionID: "telegram:1:2",
		UserID:    "999",
		ChatID:    "1",
		Text:      "cron",
		ID:        "cron-319-1",
	}); err != nil {
		t.Fatal(err)
	}
	raw := <-fc.writes
	var out outboundFrame
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "push" || out.UserID != "1182" || out.ID != "cron-319-1" {
		t.Fatalf("%+v", out)
	}
}

func TestPush_LiveWriteFallsBackToDial(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.setLive(&fakeConn{writeErr: io.ErrClosedPipe, writes: make(chan []byte, 1)})
	good := &fakeConn{writes: make(chan []byte, 1)}
	ch.dial = func(context.Context, string, http.Header) (conn, error) {
		return good, nil
	}
	if err := ch.Push(context.Background(), channel.Outbound{Text: "cron"}); err != nil {
		t.Fatal(err)
	}
	raw := <-good.writes
	var out outboundFrame
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "push" || out.UserID != "1182" || out.ID == "" {
		t.Fatalf("%+v", out)
	}
}

// The crane's own mailbox socket died while the model was working. Drafts
// and typing may fail silently; the reply must land on a fresh dial.
func TestDispatch_ReplyFallsBackToDialWhenSocketDies(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", streaming), func(t *testing.T) {
			ch, err := New(Config{
				MailboxURL:    "wss://x.workers.dev/ws/kit",
				Bearer:        "tok",
				AllowedUsers:  []string{"1182"},
				StreamReplies: streaming,
			})
			if err != nil {
				t.Fatal(err)
			}
			dead := &fakeConn{reads: make(chan []byte, 1), writeErr: io.ErrClosedPipe}
			good := &fakeConn{writes: make(chan []byte, 4)}
			dials := 0
			ch.dial = func(context.Context, string, http.Header) (conn, error) {
				dials++
				return good, nil
			}
			raw, _ := json.Marshal(inboundFrame{Text: "hi", UserID: "1182"})
			err = ch.dispatch(context.Background(), dead, raw, func(ctx context.Context, _ channel.Message) (string, error) {
				if w, ok := channel.ReplyWriterFrom(ctx); ok {
					_ = w.(channel.StatusWriter).UpdateStatus(ctx, "⏳ spinning up")
				}
				return "Sleep score 74.", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			out := recvReply(t, good.writes)
			if out.Kind != "reply" || out.Text != "Sleep score 74." || out.UserID != "1182" {
				t.Fatalf("dialed reply = %+v", out)
			}
			if dials != 1 {
				t.Fatalf("dials = %d, want 1 (drafts and typing must not dial)", dials)
			}
		})
	}
}

func TestIsAllowed_SubOrEmailOrNeither(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182:ada@example.com", "bob@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ch.isAllowed("1182", "") {
		t.Fatal("sub")
	}
	if !ch.isAllowed("", "  ADA@example.com ") {
		t.Fatal("email")
	}
	if !ch.isAllowed("999", "bob@example.com") {
		t.Fatal("email-only row")
	}
	if ch.isAllowed("999", "eve@example.com") {
		t.Fatal("neither")
	}
}

func TestDispatch_EmailOnlyMatchKeepsSub(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"ada@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	raw, _ := json.Marshal(inboundFrame{Text: "hi", UserID: "1182999", Email: "Ada@Example.com"})
	if err := ch.dispatch(context.Background(), fc, raw, func(_ context.Context, msg channel.Message) (string, error) {
		if msg.UserID != "1182999" {
			t.Fatalf("userid %q", msg.UserID)
		}
		if msg.SessionID != channel.AgentSession {
			t.Fatalf("sid %q", msg.SessionID)
		}
		if msg.ChatID != "1182999" {
			t.Fatalf("chatid %q", msg.ChatID)
		}
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}
	out := recvReply(t, fc.writes)
	if out.Kind != "reply" || out.UserID != "1182999" {
		t.Fatalf("%+v", out)
	}

	called := false
	raw, _ = json.Marshal(inboundFrame{Text: "hi", UserID: "1", Email: "eve@example.com"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		called = true
		return "nope", nil
	}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("neither must drop")
	}
}

func TestDispatch_EmailOnlyLearnsSubForPushAndAdmit(t *testing.T) {
	var admits []string
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"ada@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.SetOnAdmit(func(_ context.Context, sid, uid string) {
		admits = append(admits, sid+"|"+uid)
	})
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 16)}
	ch.setLive(fc)
	if err := ch.Push(context.Background(), channel.Outbound{Text: "early"}); err != nil {
		t.Fatal(err)
	}
	early := recvOutbound(t, fc.writes)
	if early.Kind != "push" || early.UserID != "" {
		t.Fatalf("email-only broadcast %+v", early)
	}
	raw, _ := json.Marshal(inboundFrame{Text: "hi", UserID: "1182999", Email: "ada@example.com"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = recvReply(t, fc.writes)
	if len(admits) != 1 || admits[0] != channel.AgentSession+"|1182999" {
		t.Fatalf("admits=%v", admits)
	}

	if err := ch.Push(context.Background(), channel.Outbound{UserID: "1182999", Text: "wake"}); err != nil {
		t.Fatal(err)
	}
	out := recvOutbound(t, fc.writes)
	for out.Kind == "typing" {
		out = recvOutbound(t, fc.writes)
	}
	if out.Kind != "push" || out.UserID != "1182999" || out.Text != "wake" {
		t.Fatalf("push %+v", out)
	}

	raw, _ = json.Marshal(inboundFrame{Text: "again", UserID: "1182999", Email: "ada@example.com"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		return "ok2", nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(admits) != 1 {
		t.Fatalf("admit should be once: %v", admits)
	}
}

func TestDispatch_SilentPinLearnsSub(t *testing.T) {
	var admits int
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"ada@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.SetOnAdmit(func(context.Context, string, string) { admits++ })
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 1)}
	ch.setLive(fc)
	raw, _ := json.Marshal(inboundFrame{
		UserID:  "1182999",
		Email:   "ada@example.com",
		Context: &frameContext{Geo: &geo{Lat: 1, Lon: 2}},
	})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		t.Fatal("silent pin")
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if admits != 1 {
		t.Fatalf("admits=%d", admits)
	}
	if err := ch.Push(context.Background(), channel.Outbound{UserID: "1182999", Text: "wake"}); err != nil {
		t.Fatal(err)
	}
}

func TestTrustSub_AllowsPushWithoutInbound(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"ada@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.TrustSub("1182999")
	fc := &fakeConn{writes: make(chan []byte, 1)}
	ch.setLive(fc)
	if err := ch.Push(context.Background(), channel.Outbound{Text: "wake"}); err != nil {
		t.Fatal(err)
	}
	out := recvOutbound(t, fc.writes)
	if out.Kind != "push" || out.UserID != "1182999" || out.Text != "wake" {
		t.Fatalf("push %+v", out)
	}
}

func TestServe_PublishesCatalog(t *testing.T) {
	var logs bytes.Buffer
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182:ada@example.com", "bob@example.com"},
		Logger:       slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte), writes: make(chan []byte, 2)}
	ch.dial = func(context.Context, string, http.Header) (conn, error) {
		return fc, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ch.serve(ctx, func(context.Context, channel.Message) (string, error) {
			t.Error("handle")
			return "", nil
		})
	}()
	var frames []outboundFrame
	for i := 0; i < 2; i++ {
		select {
		case raw := <-fc.writes:
			var out outboundFrame
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			frames = append(frames, out)
		case <-time.After(2 * time.Second):
			t.Fatalf("missing frame %d", i)
		}
	}
	cancel()
	_ = fc.Close()
	<-done
	if frames[0].Kind != "cmds" {
		t.Fatalf("first %+v", frames[0])
	}
	names := map[string]struct{}{}
	for _, c := range frames[0].Commands {
		names[c.Name] = struct{}{}
	}
	if _, ok := names["new"]; !ok {
		t.Fatalf("missing new: %+v", frames[0].Commands)
	}
	if _, ok := names["brief"]; !ok {
		t.Fatalf("missing brief: %+v", frames[0].Commands)
	}
	if frames[1].Kind != "allow" || len(frames[1].Users) != 2 {
		t.Fatalf("allow %+v", frames[1])
	}
	if frames[1].Users[0].Sub != "1182" || frames[1].Users[0].Email != "ada@example.com" {
		t.Fatalf("users[0] %+v", frames[1].Users[0])
	}
	if frames[1].Users[1].Sub != "" || frames[1].Users[1].Email != "bob@example.com" {
		t.Fatalf("users[1] %+v", frames[1].Users[1])
	}
	if !strings.Contains(logs.String(), "pendant allow sent") {
		t.Fatalf("log = %s", logs.String())
	}
}

func TestDispatch_MCPPhotoSink(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	raw, _ := json.Marshal(inboundFrame{Text: "draw a red bike", UserID: "1182"})
	if err := ch.dispatch(context.Background(), fc, raw, func(ctx context.Context, _ channel.Message) (string, error) {
		if s := channel.PhotoSinkFrom(ctx); s != nil {
			s.Add("data:image/png;base64,AQID")
		} else {
			t.Fatal("PhotoSink missing on Handle ctx")
		}
		return "drew a red bike", nil
	}); err != nil {
		t.Fatal(err)
	}
	out := recvReply(t, fc.writes)
	if out.Kind != "reply" || out.Text != "drew a red bike" || out.UserID != "1182" {
		t.Fatalf("%+v", out)
	}
	if len(out.Images) != 1 || out.Images[0].URL != "data:image/png;base64,AQID" {
		t.Fatalf("images %+v", out.Images)
	}
}

func TestDispatch_PhotoOnlyEmptyText(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	raw, _ := json.Marshal(inboundFrame{Text: "draw", UserID: "1182"})
	if err := ch.dispatch(context.Background(), fc, raw, func(ctx context.Context, _ channel.Message) (string, error) {
		channel.PhotoSinkFrom(ctx).Add("data:image/png;base64,AQID")
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	out := recvReply(t, fc.writes)
	if out.Kind != "reply" || out.Text != "" || len(out.Images) != 1 {
		t.Fatalf("photo-only %+v", out)
	}
}

func TestPush_IncludesPhotos(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte), writes: make(chan []byte, 1)}
	ch.setLive(fc)
	if err := ch.Push(context.Background(), channel.Outbound{
		Text:   "drew it",
		Photos: []string{"data:image/png;base64,AQID"},
	}); err != nil {
		t.Fatal(err)
	}
	raw := <-fc.writes
	var out outboundFrame
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "push" || out.Text != "drew it" || out.UserID != "1182" {
		t.Fatalf("%+v", out)
	}
	if len(out.Images) != 1 || out.Images[0].URL != "data:image/png;base64,AQID" {
		t.Fatalf("images %+v", out.Images)
	}
}

func TestDispatch_WorkerPhotoJSONAndEmpty(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	raw := []byte(`{"kind":"inbound","user_id":"1182","images":[{"url":"data:image/jpeg;base64,aa"}]}`)
	if err := ch.dispatch(context.Background(), fc, raw, func(_ context.Context, msg channel.Message) (string, error) {
		if msg.Text != "[photo]" {
			t.Fatalf("text %q", msg.Text)
		}
		if len(msg.Images) != 1 || msg.Images[0].URL != "data:image/jpeg;base64,aa" {
			t.Fatalf("images %+v", msg.Images)
		}
		return "saw photo", nil
	}); err != nil {
		t.Fatal(err)
	}
	out := recvReply(t, fc.writes)
	if out.Kind != "reply" || out.Text != "saw photo" || out.UserID != "1182" {
		t.Fatalf("%+v", out)
	}

	called := false
	raw = []byte(`{"kind":"pin","user_id":"1182"}`)
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		called = true
		return "nope", nil
	}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("empty pin must not start a turn")
	}
	select {
	case <-fc.writes:
		t.Fatal("no reply on empty inbound")
	default:
	}
}

func TestDispatch_TypesThenReply(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	raw, _ := json.Marshal(inboundFrame{Text: "hi", UserID: "1182"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}
	first := recvOutbound(t, fc.writes)
	if first.Kind != "typing" || first.UserID != "1182" || first.Text != "" {
		t.Fatalf("first %+v", first)
	}
	out := recvReply(t, fc.writes)
	if out.Kind != "reply" || out.Text != "ok" || out.UserID != "1182" {
		t.Fatalf("%+v", out)
	}
}

func TestDispatch_TypingRefresh(t *testing.T) {
	prev := typingEvery
	typingEvery = 20 * time.Millisecond
	t.Cleanup(func() { typingEvery = prev })

	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	gate := make(chan struct{})
	done := make(chan error, 1)
	raw, _ := json.Marshal(inboundFrame{Text: "hi", UserID: "1182"})
	go func() {
		done <- ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
			<-gate
			return "ok", nil
		})
	}()
	n := 0
	deadline := time.After(500 * time.Millisecond)
	for n < 2 {
		select {
		case rawOut := <-fc.writes:
			var out outboundFrame
			if err := json.Unmarshal(rawOut, &out); err != nil {
				t.Fatal(err)
			}
			if out.Kind != "typing" || out.UserID != "1182" {
				t.Fatalf("refresh %+v", out)
			}
			n++
		case <-deadline:
			t.Fatalf("typing count %d", n)
		}
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	out := recvReply(t, fc.writes)
	if out.Kind != "reply" || out.Text != "ok" {
		t.Fatalf("%+v", out)
	}
}

func TestDispatch_IgnoresTypingFrame(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	called := false
	raw, _ := json.Marshal(inboundFrame{Kind: "typing", UserID: "1182"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		called = true
		return "nope", nil
	}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("inbound typing must not start a turn")
	}
	select {
	case <-fc.writes:
		t.Fatal("no write on inbound typing")
	default:
	}
}

func TestAimsFrame_GoldenRoundTrip(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "aims_frame.json"))
	if err != nil {
		t.Fatal(err)
	}
	var frame outboundFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatal(err)
	}
	if frame.Kind != "aims" || frame.UserID != "" || frame.Aims == nil || len(*frame.Aims) != 1 {
		t.Fatalf("frame %+v", frame)
	}
	row := (*frame.Aims)[0]
	if row.Area != "training" || row.Sentence != "gym 3 mornings/wk" || row.Note != "asked" || row.NoteAt != "2026-09-25" {
		t.Fatalf("row %+v", row)
	}
	if len(row.Days) != 5 || row.Days[2].Score != 0 || len(row.Days[2].Events) != 0 || row.Days[0].Events[0] != 411 {
		t.Fatalf("days %+v", row.Days)
	}
	if row.Slope == nil || *row.Slope != 0.3 || row.Block == nil || row.Block.Up != 4 || row.Block.Pct != 0.4 {
		t.Fatalf("slope=%v block=%+v", row.Slope, row.Block)
	}
	if row.Effect == nil || row.Effect.R != -0.42 || row.Effect.Metric != "weight" || len(row.Weeks) != 2 || len(row.Weeks[0].Metrics) != 0 {
		t.Fatalf("effect=%+v weeks=%+v", row.Effect, row.Weeks)
	}
	if len(frame.Links) != 1 || frame.Links[0].A != "training" || frame.Links[0].N != 12 {
		t.Fatalf("links %+v", frame.Links)
	}
	again, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	var back outboundFrame
	if err := json.Unmarshal(again, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(frame, back) {
		t.Fatalf("round trip\n%+v\n%+v", frame, back)
	}
	if strings.Contains(string(again), `"user_id"`) {
		t.Fatalf("board is room-wide: %s", again)
	}
}

func TestAimsFrame_EmptyRowsStillSent(t *testing.T) {
	raw, err := json.Marshal(aimsFrame(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `"aims":[]`) || strings.Contains(body, `"links"`) || strings.Contains(body, `"user_id"`) {
		t.Fatalf("empty board: %s", body)
	}
}

func TestPush_AimsOnlyWhenBoardChanges(t *testing.T) {
	ch := testPendant(t)
	ch.board = func(context.Context) ([]aims.Row, []aims.Link, error) {
		return []aims.Row{{
			Area: "training", Sentence: "gym",
			Days: []aims.DayCell{{Day: "2026-09-26", Events: []int64{}}},
		}}, nil, nil
	}
	fc := &fakeConn{writes: make(chan []byte, 8)}
	ch.setLive(fc)
	msg := channel.Outbound{Text: "wake"}
	if err := ch.Push(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	got := drainFrames(fc.writes)
	if len(got) != 2 || got[0].Kind != "push" || got[1].Kind != "aims" {
		t.Fatalf("first push %+v", kinds(got))
	}
	if err := ch.Push(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	got = drainFrames(fc.writes)
	if len(got) != 1 || got[0].Kind != "push" {
		t.Fatalf("unchanged push %+v", kinds(got))
	}
}

func TestServe_AimsBetweenCmdsAndAllow(t *testing.T) {
	ch := testPendant(t)
	ch.board = func(context.Context) ([]aims.Row, []aims.Link, error) {
		return []aims.Row{{Area: "training", Sentence: "gym", Days: []aims.DayCell{{Day: "2026-09-27", Events: []int64{}}}}}, nil, nil
	}
	frames := serveOpening(t, ch, 3)
	if frames[0].Kind != "cmds" || frames[1].Kind != "aims" || frames[2].Kind != "allow" {
		t.Fatalf("order %s %s %s", frames[0].Kind, frames[1].Kind, frames[2].Kind)
	}
	if frames[1].UserID != "" || frames[1].Aims == nil || len(*frames[1].Aims) != 1 {
		t.Fatalf("aims %+v", frames[1])
	}
}

func TestRunTurn_AimsOnlyWhenBoardChanges(t *testing.T) {
	ctx := context.Background()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	mem, err := memory.OpenDB(sess.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindInsight, "aim/training", "gym 3 mornings"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindInsight, "aim/weight", "lose weight"); err != nil {
		t.Fatal(err)
	}
	store, err := aims.OpenDB(sess.DB(), loc, mem)
	if err != nil {
		t.Fatal(err)
	}
	ch := testPendant(t)
	now := time.Date(2026, 9, 27, 8, 0, 0, 0, loc)
	ch.board = func(ctx context.Context) ([]aims.Row, []aims.Link, error) {
		areas, err := store.Areas(ctx)
		if err != nil {
			return nil, nil, err
		}
		return store.Board(ctx, areas, now)
	}
	fc := &fakeConn{writes: make(chan []byte, 16)}
	msg := channel.Message{UserID: "1182", Text: "hi"}
	if err := ch.runTurn(ctx, fc, func(context.Context, channel.Message) (string, error) {
		_, err := store.Log(ctx, aims.Event{What: "gym", Day: "2026-09-26"}, map[string]int{"training": 2})
		return "logged", err
	}, msg, ""); err != nil {
		t.Fatal(err)
	}
	got := drainFrames(fc.writes)
	aimsN := countKind(got, "aims")
	if aimsN != 1 || !frameHasArea(got, "training") {
		t.Fatalf("after log: %+v", kinds(got))
	}
	if err := ch.runTurn(ctx, fc, func(context.Context, channel.Message) (string, error) {
		return "same", nil
	}, msg, ""); err != nil {
		t.Fatal(err)
	}
	got = drainFrames(fc.writes)
	if countKind(got, "aims") != 0 {
		t.Fatalf("unchanged turn wrote aims: %+v", kinds(got))
	}
	if err := ch.runTurn(ctx, fc, func(context.Context, channel.Message) (string, error) {
		row, ok, err := mem.ActiveByKindSubject(ctx, memory.KindInsight, "aim/training")
		if err != nil || !ok {
			return "", err
		}
		if err := mem.Forget(ctx, row.ID); err != nil {
			return "", err
		}
		return "forgot", store.Forget(ctx, "training")
	}, msg, ""); err != nil {
		t.Fatal(err)
	}
	got = drainFrames(fc.writes)
	if countKind(got, "aims") != 1 || frameHasArea(got, "training") {
		t.Fatalf("after forget: %+v", kinds(got))
	}
}

func testPendant(t *testing.T) *Channel {
	t.Helper()
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func serveOpening(t *testing.T, ch *Channel, n int) []outboundFrame {
	t.Helper()
	fc := &fakeConn{reads: make(chan []byte), writes: make(chan []byte, n)}
	ch.dial = func(context.Context, string, http.Header) (conn, error) { return fc, nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ch.serve(ctx, func(context.Context, channel.Message) (string, error) { return "", nil })
	}()
	var frames []outboundFrame
	for i := 0; i < n; i++ {
		select {
		case raw := <-fc.writes:
			var out outboundFrame
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			frames = append(frames, out)
		case <-time.After(2 * time.Second):
			t.Fatalf("missing frame %d", i)
		}
	}
	cancel()
	_ = fc.Close()
	<-done
	return frames
}

func drainFrames(writes <-chan []byte) []outboundFrame {
	var out []outboundFrame
	for {
		select {
		case raw := <-writes:
			var frame outboundFrame
			if json.Unmarshal(raw, &frame) == nil {
				out = append(out, frame)
			}
		default:
			return out
		}
	}
}

func countKind(frames []outboundFrame, kind string) int {
	n := 0
	for _, f := range frames {
		if f.Kind == kind {
			n++
		}
	}
	return n
}

func frameHasArea(frames []outboundFrame, area string) bool {
	for _, f := range frames {
		if f.Aims == nil {
			continue
		}
		for _, row := range *f.Aims {
			if row.Area == area {
				return true
			}
		}
	}
	return false
}

func kinds(frames []outboundFrame) []string {
	out := make([]string, len(frames))
	for i, f := range frames {
		out[i] = f.Kind
	}
	return out
}
