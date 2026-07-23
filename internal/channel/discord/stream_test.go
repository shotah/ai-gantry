package discord

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

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

func TestEditStream_SendAndEdit(t *testing.T) {
	mock := &mockSession{}
	s := newEditStream(mock, "dm", 2000)
	ctx := context.Background()
	if err := s.Update(ctx, "hi"); err != nil {
		t.Fatal(err)
	}
	if !s.Started() || s.msgID == "" {
		t.Fatal("not started")
	}
	time.Sleep(streamMinEditGap)
	if err := s.Update(ctx, "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, "hello world final"); err != nil {
		t.Fatal(err)
	}
	if len(mock.msgs) < 1 {
		t.Fatal("no send")
	}
	if len(mock.edits) < 1 {
		t.Fatal("no edits")
	}
}

func TestChannel_StreamRepliesUsesWriter(t *testing.T) {
	mock := &mockSession{botID: "bot1"}
	ch, err := New(Config{Token: "tok", AllowedUsers: []string{"42"}, StreamReplies: true})
	if err != nil {
		t.Fatal(err)
	}
	ch.newSession = func(string) (session, error) { return mock, nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ch.Run(ctx, func(ctx context.Context, _ channel.Message) (string, error) {
			w, ok := channel.ReplyWriterFrom(ctx)
			if !ok {
				t.Error("missing ReplyWriter")
				return "", nil
			}
			_ = w.Update(ctx, "partial")
			time.Sleep(streamMinEditGap)
			_ = w.Update(ctx, "done")
			return "done", nil
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(mock.handlers) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	var msgHandler func(*discordgo.Session, *discordgo.MessageCreate)
	for _, h := range mock.handlers {
		if fn, ok := h.(func(*discordgo.Session, *discordgo.MessageCreate)); ok {
			msgHandler = fn
		}
	}
	if msgHandler == nil {
		t.Fatal("no handler")
	}
	msgHandler(nil, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "dmchan",
			Content:   "hi",
			Author:    &discordgo.User{ID: "42"},
		},
	})

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mock.mu.Lock()
		nEdit := len(mock.edits)
		mock.mu.Unlock()
		if nEdit >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mock.mu.Lock()
	if len(mock.msgs) < 1 {
		t.Fatalf("msgs=%v edits=%v", mock.msgs, mock.edits)
	}
	mock.mu.Unlock()

	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit")
	}
}
