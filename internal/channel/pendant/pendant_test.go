package pendant

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/here"
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
	reads  chan []byte
	writes chan []byte
	once   sync.Once
}

func (f *fakeConn) ReadMessage() (int, []byte, error) {
	b, ok := <-f.reads
	if !ok {
		return 0, nil, io.EOF
	}
	return 1, b, nil
}

func (f *fakeConn) WriteMessage(_ int, data []byte) error {
	f.writes <- append([]byte(nil), data...)
	return nil
}

func (f *fakeConn) Close() error {
	f.once.Do(func() { close(f.reads) })
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
		if msg.SessionID != "pendant:kit:1182" {
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
	p, ok := here.Get("pendant:kit:1182")
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
	if err := ch.Push(context.Background(), channel.Outbound{UserID: "nope", Text: "x"}); err == nil {
		t.Fatal("deny")
	}
	fc := &fakeConn{reads: make(chan []byte), writes: make(chan []byte, 1)}
	ch.setLive(fc)
	if err := ch.Push(context.Background(), channel.Outbound{UserID: "1182", Text: "ping"}); err != nil {
		t.Fatal(err)
	}
	raw := <-fc.writes
	if !strings.Contains(string(raw), `"user_id":"1182"`) {
		t.Fatalf("push json %s", raw)
	}
	var out outboundFrame
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "push" || out.Text != "ping" || out.UserID != "1182" {
		t.Fatalf("%+v", out)
	}
}

func TestPush_BroadcastOmitsUserID(t *testing.T) {
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
	if err := ch.Push(context.Background(), channel.Outbound{ChatID: "1182", Text: "cron"}); err != nil {
		t.Fatal(err)
	}
	raw := <-fc.writes
	if !strings.Contains(string(raw), `"user_id":"1182"`) {
		t.Fatalf("chatid push json %s", raw)
	}
	var out outboundFrame
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "push" || out.UserID != "1182" {
		t.Fatalf("%+v", out)
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
		if msg.SessionID != "pendant:kit:1182999" {
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
	if err := ch.Push(context.Background(), channel.Outbound{UserID: "1182999", Text: "early"}); err == nil {
		t.Fatal("push must wait until the sub is learned")
	}

	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 16)}
	ch.setLive(fc)
	raw, _ := json.Marshal(inboundFrame{Text: "hi", UserID: "1182999", Email: "ada@example.com"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = recvReply(t, fc.writes)
	if len(admits) != 1 || admits[0] != "pendant:kit:1182999|1182999" {
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
	if err := ch.Push(context.Background(), channel.Outbound{UserID: "1182999", Text: "early"}); err == nil {
		t.Fatal("push must wait until TrustSub")
	}
	ch.TrustSub("1182999")
	fc := &fakeConn{writes: make(chan []byte, 1)}
	ch.setLive(fc)
	if err := ch.Push(context.Background(), channel.Outbound{UserID: "1182999", Text: "wake"}); err != nil {
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
