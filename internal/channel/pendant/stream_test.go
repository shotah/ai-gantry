package pendant

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

type captureWriter struct {
	mu     sync.Mutex
	frames []outboundFrame
}

func (c *captureWriter) write(frame outboundFrame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frames = append(c.frames, frame)
	return nil
}

func (c *captureWriter) all() []outboundFrame {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]outboundFrame, len(c.frames))
	copy(out, c.frames)
	return out
}

func TestEditStream_StatusAndProgressThenReply(t *testing.T) {
	w := &captureWriter{}
	s := newEditStream(w.write, "1182")
	ctx := context.Background()

	if err := s.UpdateStatus(ctx, "⏳ spinning up"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateProgress(ctx, "Making Calls:"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateProgress(ctx, "✓"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStatus(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, "You rode 21mi."); err != nil {
		t.Fatal(err)
	}

	frames := w.all()
	if len(frames) < 2 {
		t.Fatalf("frames = %+v", frames)
	}
	if frames[0].Kind != "draft" || !strings.Contains(frames[0].Text, "spinning up") || frames[0].UserID != "1182" {
		t.Fatalf("first %+v", frames[0])
	}
	var lastDraft, reply outboundFrame
	for _, f := range frames {
		switch f.Kind {
		case "draft":
			lastDraft = f
		case "reply":
			reply = f
		}
	}
	if !strings.Contains(lastDraft.Text, "Making Calls: ✓") {
		t.Fatalf("last draft %+v", lastDraft)
	}
	if strings.Contains(reply.Text, "spinning up") {
		t.Fatalf("status survived into reply: %+v", reply)
	}
	if !strings.Contains(reply.Text, "Making Calls: ✓") || !strings.Contains(reply.Text, "You rode 21mi.") {
		t.Fatalf("reply %+v", reply)
	}
	if reply.Kind != "reply" || reply.UserID != "1182" {
		t.Fatalf("reply %+v", reply)
	}
}

func TestEditStream_FinishDropsLingeringStatus(t *testing.T) {
	w := &captureWriter{}
	s := newEditStream(w.write, "1182")
	ctx := context.Background()
	if err := s.UpdateStatus(ctx, "⏳ spinning up"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, "model call failed"); err != nil {
		t.Fatal(err)
	}
	frames := w.all()
	reply := frames[len(frames)-1]
	if reply.Kind != "reply" || reply.Text != "model call failed" {
		t.Fatalf("%+v", frames)
	}
}

func TestEditStream_DiscardClearsDraft(t *testing.T) {
	w := &captureWriter{}
	s := newEditStream(w.write, "1182")
	ctx := context.Background()
	if err := s.UpdateStatus(ctx, "⏳ spinning up"); err != nil {
		t.Fatal(err)
	}
	if !s.Started() {
		t.Fatal("started")
	}
	if err := s.Discard(ctx); err != nil {
		t.Fatal(err)
	}
	frames := w.all()
	last := frames[len(frames)-1]
	if last.Kind != "draft" || last.Text != "" || last.UserID != "1182" {
		t.Fatalf("discard %+v", last)
	}
	if s.Started() {
		t.Fatal("discard should un-start")
	}
}

func TestEditStream_UpdateThrottled(t *testing.T) {
	prev := streamMinGap
	streamMinGap = 20 * time.Millisecond
	t.Cleanup(func() { streamMinGap = prev })

	w := &captureWriter{}
	s := newEditStream(w.write, "1182")
	ctx := context.Background()
	if err := s.Update(ctx, "Hel"); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, "Hell"); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, "Hello"); err != nil {
		t.Fatal(err)
	}
	if n := countDrafts(w.all()); n != 1 {
		t.Fatalf("drafts = %d want 1 (one per gap): %+v", n, w.all())
	}
	if w.all()[0].Text != "Hel" {
		t.Fatalf("leading draft %+v", w.all()[0])
	}
	drafts := waitDrafts(t, w, 2, 300*time.Millisecond)
	if drafts[len(drafts)-1].Text != "Hello" {
		t.Fatalf("trailing draft %+v", drafts)
	}
	if err := s.Finish(ctx, "Hello"); err != nil {
		t.Fatal(err)
	}
	last := w.all()[len(w.all())-1]
	if last.Kind != "reply" || last.Text != "Hello" {
		t.Fatalf("finish %+v", last)
	}
}

func TestEditStream_FinishCancelsTrailingDraft(t *testing.T) {
	prev := streamMinGap
	streamMinGap = time.Hour
	t.Cleanup(func() { streamMinGap = prev })

	w := &captureWriter{}
	s := newEditStream(w.write, "1182")
	ctx := context.Background()
	if err := s.Update(ctx, "Hel"); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, "Hello"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, "Hello"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	frames := w.all()
	replyAt := -1
	for i, f := range frames {
		if f.Kind == "reply" {
			replyAt = i
			break
		}
	}
	if replyAt < 0 {
		t.Fatalf("missing reply: %+v", frames)
	}
	if frames[replyAt].Text != "Hello" {
		t.Fatalf("reply %+v", frames[replyAt])
	}
	for i, f := range frames {
		if i > replyAt && f.Kind == "draft" {
			t.Fatalf("draft after reply: %+v", frames)
		}
	}
}

func countDrafts(frames []outboundFrame) int {
	n := 0
	for _, f := range frames {
		if f.Kind == "draft" {
			n++
		}
	}
	return n
}

func waitDrafts(t *testing.T, w *captureWriter, n int, d time.Duration) []outboundFrame {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		all := w.all()
		var drafts []outboundFrame
		for _, f := range all {
			if f.Kind == "draft" {
				drafts = append(drafts, f)
			}
		}
		if len(drafts) >= n {
			return drafts
		}
		if time.Now().After(deadline) {
			t.Fatalf("want %d drafts, got %d: %+v", n, len(drafts), all)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEditStream_HidesWaitToken(t *testing.T) {
	prev := streamMinGap
	streamMinGap = 0
	t.Cleanup(func() { streamMinGap = prev })

	w := &captureWriter{}
	s := newEditStream(w.write, "1182")
	ctx := context.Background()
	if err := s.Update(ctx, "Thai or pizza?\n[wait]"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, "Thai or pizza?"); err != nil {
		t.Fatal(err)
	}
	for _, f := range w.all() {
		if strings.Contains(strings.ToLower(f.Text), "[wait]") {
			t.Fatalf("leaked [wait]: %+v", f)
		}
	}
	last := w.all()[len(w.all())-1]
	if last.Kind != "reply" || last.Text != "Thai or pizza?" {
		t.Fatalf("finish %+v", last)
	}
}

func TestDispatch_StreamDraftThenReply(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:    "wss://x.workers.dev/ws/kit",
		Bearer:        "tok",
		AllowedUsers:  []string{"1182"},
		StreamReplies: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 16)}
	raw, _ := json.Marshal(inboundFrame{Text: "hi", UserID: "1182"})
	if err := ch.dispatch(context.Background(), fc, raw, func(ctx context.Context, _ channel.Message) (string, error) {
		w, ok := channel.ReplyWriterFrom(ctx)
		if !ok {
			t.Fatal("missing writer")
		}
		if err := w.(channel.StatusWriter).UpdateStatus(ctx, "⏳ spinning up"); err != nil {
			t.Fatal(err)
		}
		if err := w.(channel.ProgressWriter).UpdateProgress(ctx, "Making Calls:"); err != nil {
			t.Fatal(err)
		}
		if err := w.(channel.ProgressWriter).UpdateProgress(ctx, "✓"); err != nil {
			t.Fatal(err)
		}
		return "done", nil
	}); err != nil {
		t.Fatal(err)
	}

	var drafts []outboundFrame
	var reply outboundFrame
	for {
		out := recvOutbound(t, fc.writes)
		switch out.Kind {
		case "typing":
			continue
		case "draft":
			drafts = append(drafts, out)
		case "reply":
			reply = out
			if len(drafts) == 0 {
				t.Fatal("want draft before reply")
			}
			if !strings.Contains(drafts[0].Text, "spinning up") {
				t.Fatalf("first draft %+v", drafts[0])
			}
			if strings.Contains(reply.Text, "spinning up") {
				t.Fatalf("status in reply %+v", reply)
			}
			if !strings.Contains(reply.Text, "Making Calls: ✓") || reply.Text == "" || !strings.Contains(reply.Text, "done") {
				t.Fatalf("reply %+v", reply)
			}
			if reply.UserID != "1182" {
				t.Fatalf("userid %+v", reply)
			}
			return
		default:
			t.Fatalf("unexpected %+v", out)
		}
	}
}

func TestEditStream_FinishCarriesPhoto(t *testing.T) {
	w := &captureWriter{}
	s := newEditStream(w.write, "1182")
	s.setPhotos([]string{"data:image/png;base64,AQID"})
	if err := s.Finish(context.Background(), "drew a red bike"); err != nil {
		t.Fatal(err)
	}
	frames := w.all()
	if len(frames) != 1 {
		t.Fatalf("%+v", frames)
	}
	if frames[0].Kind != "reply" || frames[0].Text != "drew a red bike" {
		t.Fatalf("%+v", frames[0])
	}
	if len(frames[0].Images) != 1 || frames[0].Images[0].URL != "data:image/png;base64,AQID" {
		t.Fatalf("images %+v", frames[0].Images)
	}
}

func TestDispatch_StreamReplyCarriesPhoto(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:    "wss://x.workers.dev/ws/kit",
		Bearer:        "tok",
		AllowedUsers:  []string{"1182"},
		StreamReplies: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 16)}
	raw, _ := json.Marshal(inboundFrame{Text: "draw", UserID: "1182"})
	if err := ch.dispatch(context.Background(), fc, raw, func(ctx context.Context, _ channel.Message) (string, error) {
		w, _ := channel.ReplyWriterFrom(ctx)
		_ = w.(channel.StatusWriter).UpdateStatus(ctx, "⏳ spinning up")
		channel.PhotoSinkFrom(ctx).Add("data:image/png;base64,AQID")
		return "drew a red bike", nil
	}); err != nil {
		t.Fatal(err)
	}
	for {
		out := recvOutbound(t, fc.writes)
		switch out.Kind {
		case "typing", "draft":
			continue
		case "reply":
			if out.Text == "" || !strings.Contains(out.Text, "drew a red bike") {
				t.Fatalf("reply %+v", out)
			}
			if len(out.Images) != 1 || out.Images[0].URL != "data:image/png;base64,AQID" {
				t.Fatalf("images %+v", out.Images)
			}
			return
		default:
			t.Fatalf("unexpected %+v", out)
		}
	}
}

func TestDispatch_EmptyReplyDiscardsDraft(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:    "wss://x.workers.dev/ws/kit",
		Bearer:        "tok",
		AllowedUsers:  []string{"1182"},
		StreamReplies: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 16)}
	raw, _ := json.Marshal(inboundFrame{Text: "hi", UserID: "1182"})
	if err := ch.dispatch(context.Background(), fc, raw, func(ctx context.Context, _ channel.Message) (string, error) {
		w, _ := channel.ReplyWriterFrom(ctx)
		_ = w.(channel.StatusWriter).UpdateStatus(ctx, "⏳ spinning up")
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	sawEmpty := false
	deadline := time.After(2 * time.Second)
	for !sawEmpty {
		select {
		case rawOut := <-fc.writes:
			var out outboundFrame
			if err := json.Unmarshal(rawOut, &out); err != nil {
				t.Fatal(err)
			}
			if out.Kind == "reply" {
				t.Fatalf("empty cancel must not reply: %+v", out)
			}
			if out.Kind == "draft" && out.Text == "" {
				sawEmpty = true
			}
		case <-deadline:
			t.Fatal("missing empty draft discard")
		}
	}
}

func TestDispatch_IgnoresRoomNotices(t *testing.T) {
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	for _, kind := range []string{"face", "backdrop", "theme"} {
		called := false
		raw, _ := json.Marshal(inboundFrame{Kind: kind, Text: "1725"})
		if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
			called = true
			return "nope", nil
		}); err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatalf("%s notice must not start a turn", kind)
		}
	}
	select {
	case <-fc.writes:
		t.Fatal("no write on room notices")
	default:
	}
}

func TestDispatch_IgnoresDraftFrame(t *testing.T) {
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
	raw, _ := json.Marshal(inboundFrame{Kind: "draft", UserID: "1182", Text: "⏳"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		called = true
		return "nope", nil
	}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("inbound draft must not start a turn")
	}
	select {
	case <-fc.writes:
		t.Fatal("no write on inbound draft")
	default:
	}
}

func TestDispatch_LogsMailboxError(t *testing.T) {
	var logs bytes.Buffer
	ch, err := New(Config{
		MailboxURL:   "wss://x.workers.dev/ws/kit",
		Bearer:       "tok",
		AllowedUsers: []string{"1182"},
		Logger:       slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeConn{reads: make(chan []byte, 1), writes: make(chan []byte, 8)}
	called := false
	raw, _ := json.Marshal(inboundFrame{Kind: "error", Text: "rate", ID: "r1", UserID: "1182"})
	if err := ch.dispatch(context.Background(), fc, raw, func(context.Context, channel.Message) (string, error) {
		called = true
		return "nope", nil
	}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("mailbox error must not start a turn")
	}
	out := logs.String()
	if !strings.Contains(out, "pendant mailbox error") || !strings.Contains(out, "rate") || !strings.Contains(out, "r1") {
		t.Fatalf("log = %q", out)
	}
	select {
	case <-fc.writes:
		t.Fatal("no write on mailbox error")
	default:
	}
}
