package pendant

import (
	"context"
	"encoding/json"
	"io"
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
	if !ch.isAllowed("1182") || ch.isAllowed("1182:ada@example.com") || ch.slug != "kit" {
		t.Fatalf("slug=%q allowed=%v", ch.slug, ch.allowed)
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

func TestDispatch_GeoHereAndReply(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 1)}
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
		p, ok := here.Get(msg.SessionID)
		if !ok || p.Lat != 47.6 {
			t.Fatalf("here %+v ok=%v", p, ok)
		}
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}
	var out outboundFrame
	if err := json.Unmarshal(<-fc.writes, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "reply" || out.Text != "ok" {
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
	var out outboundFrame
	_ = json.Unmarshal(<-fc.writes, &out)
	if out.Kind != "push" || out.Text != "ping" {
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
	var out outboundFrame
	_ = json.Unmarshal(<-fc.writes, &out)
	if out.Kind != "push" {
		t.Fatalf("%+v", out)
	}
}

func TestServe_PublishesCatalog(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte), writes: make(chan []byte, 1)}
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
	var out outboundFrame
	select {
	case raw := <-fc.writes:
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no cmds frame")
	}
	cancel()
	_ = fc.Close()
	<-done
	if out.Kind != "cmds" {
		t.Fatalf("%+v", out)
	}
	names := map[string]struct{}{}
	for _, c := range out.Commands {
		names[c.Name] = struct{}{}
	}
	if _, ok := names["new"]; !ok {
		t.Fatalf("missing new: %+v", out.Commands)
	}
	if _, ok := names["brief"]; !ok {
		t.Fatalf("missing brief: %+v", out.Commands)
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
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 1)}
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
	var out outboundFrame
	if err := json.Unmarshal(<-fc.writes, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "reply" || out.Text != "saw photo" {
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
