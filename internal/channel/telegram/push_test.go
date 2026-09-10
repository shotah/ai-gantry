package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestChannel_NotifyHTML(t *testing.T) {
	m := newAPIMock(t)
	ch, err := New(Config{Token: testBotToken, AllowedUsers: []int64{42}})
	if err != nil {
		t.Fatal(err)
	}
	ch.newBot = func(token string, opts ...bot.Option) (*bot.Bot, error) {
		opts = append(opts, bot.WithServerURL(m.srv.URL), bot.WithSkipGetMe())
		return bot.New(token, opts...)
	}
	html := `🔴 <b>gantry ERROR</b> · boom` + "\n" + `<blockquote expandable>details</blockquote>`
	if err := ch.NotifyHTML(context.Background(), html); err != nil {
		t.Fatal(err)
	}
	if m.count("sendMessage") < 1 {
		t.Fatal("expected sendMessage")
	}
}

func TestChannel_Push(t *testing.T) {
	m := newAPIMock(t)
	ch, err := New(Config{Token: testBotToken, AllowedUsers: []int64{42}})
	if err != nil {
		t.Fatal(err)
	}
	ch.newBot = func(token string, opts ...bot.Option) (*bot.Bot, error) {
		opts = append(opts, bot.WithServerURL(m.srv.URL), bot.WithSkipGetMe())
		return bot.New(token, opts...)
	}
	err = ch.Push(context.Background(), channel.Outbound{
		UserID: "999",
		ChatID: "7",
		Text:   "hello from cron",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.count("sendMessage") < 1 {
		t.Fatal("expected sendMessage")
	}
}

func TestEditStream_UpdateFinish(t *testing.T) {
	prev := streamFlushEvery
	streamFlushEvery = 15 * time.Millisecond
	t.Cleanup(func() { streamFlushEvery = prev })

	m := newAPIMock(t)
	b := testBot(t, m.srv.URL)
	s := newEditStream(b, 1, 0, 100)
	ctx := context.Background()
	if err := s.Update(ctx, "Hi"); err != nil {
		t.Fatal(err)
	}
	if !s.Started() {
		t.Fatal("started")
	}
	waitMsgID(t, s)
	if err := s.Finish(ctx, "Hi there final"); err != nil {
		t.Fatal(err)
	}
	if m.count("sendMessage") < 1 {
		t.Fatal("sendMessage")
	}
	if m.count("editMessageText") < 1 {
		t.Fatal("editMessageText")
	}

	// Multi-part finish (small chunkMax).
	m2 := newAPIMock(t)
	b2 := testBot(t, m2.srv.URL)
	s2 := newEditStream(b2, 1, 0, 8)
	if err := s2.Update(ctx, "start"); err != nil {
		t.Fatal(err)
	}
	waitMsgID(t, s2)
	long := "abcdefghijklmnopqr" // > 8 runes → edit + extra SendMessage
	if err := s2.Finish(ctx, long); err != nil {
		t.Fatal(err)
	}
	if m2.count("sendMessage") < 2 {
		t.Fatalf("expected overflow send, sendMessage=%d", m2.count("sendMessage"))
	}
}

func waitMsgID(t *testing.T, s *editStream) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		s.mu.Lock()
		id := s.msgID
		s.mu.Unlock()
		if id != 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for stream message id")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
