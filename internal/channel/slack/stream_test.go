package slack

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestClipRunes(t *testing.T) {
	got := clipRunes("hello世界", 5)
	if utf8.RuneCountInString(got) != 5 {
		t.Fatalf("got=%q count=%d", got, utf8.RuneCountInString(got))
	}
	if clipRunes("abc", 10) != "abc" {
		t.Fatal("no clip")
	}
}

func TestEditStream_SendAndUpdate(t *testing.T) {
	api := &capturingPoster{}
	s := newEditStream(api, "D1", "", 3500)
	ctx := context.Background()
	if err := s.Update(ctx, "hi"); err != nil {
		t.Fatal(err)
	}
	if !s.Started() || s.ts == "" {
		t.Fatal("not started")
	}
	time.Sleep(streamMinEditGap)
	if err := s.Update(ctx, "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, "hello world final"); err != nil {
		t.Fatal(err)
	}
	if len(api.posts) < 1 {
		t.Fatal("no send")
	}
	if len(api.updates) < 1 {
		t.Fatal("no updates")
	}
}

func TestDispatch_StreamReplies(t *testing.T) {
	api := &capturingPoster{authID: "BOT"}
	ch, err := New(Config{
		BotToken:      "xoxb-1",
		AppToken:      "xapp-1",
		AllowedUsers:  []string{"U42"},
		StreamReplies: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.api = api
	ch.botUser = "BOT"

	ch.dispatch(context.Background(), "U42", "D99", "1.0", "", "hi", nil, func(ctx context.Context, _ channel.Message) (string, error) {
		w, ok := channel.ReplyWriterFrom(ctx)
		if !ok {
			t.Fatal("missing ReplyWriter")
		}
		_ = w.Update(ctx, "partial")
		time.Sleep(streamMinEditGap)
		_ = w.Update(ctx, "done")
		return "done", nil
	})
	if len(api.posts) < 1 {
		t.Fatalf("posts=%+v", api.posts)
	}
	if len(api.updates) < 1 {
		t.Fatalf("updates=%+v", api.updates)
	}
}
