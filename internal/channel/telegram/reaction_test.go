package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/shotah/ai-gantry/internal/channel"
)

func emojiReaction(emoji string) models.ReactionType {
	return models.ReactionType{
		Type: models.ReactionTypeTypeEmoji,
		ReactionTypeEmoji: &models.ReactionTypeEmoji{
			Type:  models.ReactionTypeTypeEmoji,
			Emoji: emoji,
		},
	}
}

func TestCurrentReactionLabels(t *testing.T) {
	got := currentReactionLabels([]models.ReactionType{
		emojiReaction("👍"),
		emojiReaction("👍"),
		emojiReaction("❤"),
	})
	if len(got) != 2 || got[0] != "👍" || got[1] != "❤" {
		t.Fatalf("got %v", got)
	}
}

// The agent's [react …] lands on the human's message, bare code point, and
// nothing is sent as text when the reaction was the whole reply.
func TestMakeHandler_ReactTokenSetsReaction(t *testing.T) {
	api := newAPIMock(t)
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)

	handler := ch.makeHandler(func(ctx context.Context, _ channel.Message) (string, error) {
		channel.ReactionSinkFrom(ctx).Set("❤️")
		return "", nil
	})
	handler(context.Background(), b, &models.Update{
		Message: &models.Message{
			ID:   7,
			Text: "thanks!",
			Chat: models.Chat{ID: 99, Type: "private"},
			From: &models.User{ID: 42, Username: "chris"},
		},
	})
	if api.count("setMessageReaction") != 1 {
		t.Fatalf("setMessageReaction calls = %d", api.count("setMessageReaction"))
	}
	// Multipart form: the reaction list is one JSON part, message_id another.
	body := api.body("setMessageReaction")
	if !strings.Contains(body, "\r\n7\r\n") || !strings.Contains(body, `"emoji":"❤"`) || strings.Contains(body, "\ufe0f") {
		t.Fatalf("reaction body = %s", body)
	}
	if api.count("sendMessage") != 0 {
		t.Fatal("reaction-only reply must not also send text")
	}
}

// Telegram refuses the emoji and there is no text: say it in text rather
// than nothing.
func TestMakeHandler_ReactRefusedFallsBackToText(t *testing.T) {
	api := newAPIMock(t)
	api.refuseReaction = true
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)

	handler := ch.makeHandler(func(ctx context.Context, _ channel.Message) (string, error) {
		channel.ReactionSinkFrom(ctx).Set("👍")
		return "", nil
	})
	handler(context.Background(), b, &models.Update{
		Message: &models.Message{
			ID:   7,
			Text: "thanks!",
			Chat: models.Chat{ID: 99, Type: "private"},
			From: &models.User{ID: 42, Username: "chris"},
		},
	})
	waitSendMessage(t, api, 1)
	if !strings.Contains(api.body("sendMessage"), "\r\n👍\r\n") {
		t.Fatalf("fallback text = %s", api.body("sendMessage"))
	}
}

// A settled reaction turn has no human message to react back on: no sink,
// so the kernel's text fallback applies and nothing calls setMessageReaction.
func TestMakeHandler_ReactionTurnHasNoSink(t *testing.T) {
	prev := reactionSettle
	reactionSettle = 20 * time.Millisecond
	t.Cleanup(func() { reactionSettle = prev })
	api := newAPIMock(t)
	ch := testChannel(t)
	ch.botID = 1
	ch.outbound.remember(99, 7, 0, "prior bot reply")
	b := testBot(t, api.srv.URL)

	sawSink := make(chan bool, 1)
	handler := ch.makeHandler(func(ctx context.Context, _ channel.Message) (string, error) {
		sawSink <- channel.ReactionSinkFrom(ctx) != nil
		return "noted", nil
	})
	handler(context.Background(), b, &models.Update{
		MessageReaction: &models.MessageReactionUpdated{
			Chat:        models.Chat{ID: 99, Type: "private"},
			MessageID:   7,
			User:        &models.User{ID: 42, Username: "chris"},
			NewReaction: []models.ReactionType{emojiReaction("👍")},
		},
	})
	select {
	case has := <-sawSink:
		if has {
			t.Fatal("reaction turn must not offer a reaction sink")
		}
	case <-time.After(time.Second):
		t.Fatal("settled reaction never reached the handler")
	}
}

func TestOutboundCache_RememberLookup(t *testing.T) {
	c := newOutboundCache(2)
	c.remember(1, 10, 0, "a")
	c.remember(1, 11, 3, "b")
	c.remember(1, 12, 0, "c") // evicts 10
	if _, ok := c.lookup(1, 10); ok {
		t.Fatal("expected eviction")
	}
	e, ok := c.lookup(1, 11)
	if !ok || e.text != "b" || e.threadID != 3 {
		t.Fatalf("lookup 11: %+v ok=%v", e, ok)
	}
}

func TestMakeHandler_ReactionSettlesBeforePipe(t *testing.T) {
	prev := reactionSettle
	reactionSettle = 40 * time.Millisecond
	t.Cleanup(func() { reactionSettle = prev })

	api := newAPIMock(t)
	ch := testChannel(t)
	ch.botID = 1
	ch.outbound.remember(99, 7, 0, "prior bot reply")
	b := testBot(t, api.srv.URL)

	delivered := make(chan channel.Message, 1)
	handler := ch.makeHandler(func(_ context.Context, msg channel.Message) (string, error) {
		delivered <- msg
		return "noted", nil
	})

	handler(context.Background(), b, &models.Update{
		MessageReaction: &models.MessageReactionUpdated{
			Chat:        models.Chat{ID: 99, Type: "private"},
			MessageID:   7,
			User:        &models.User{ID: 42, Username: "chris"},
			NewReaction: []models.ReactionType{emojiReaction("👍")},
		},
	})
	select {
	case <-delivered:
		t.Fatal("should not deliver before settle window")
	case <-time.After(15 * time.Millisecond):
	}

	var got channel.Message
	select {
	case got = <-delivered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for settled reaction")
	}
	want := "[reaction] 👍 on: prior bot reply"
	if got.Text != want || got.SessionID != channel.AgentSession {
		t.Fatalf("got %+v", got)
	}
	waitSendMessage(t, api, 1)
}

func TestMakeHandler_ReactionOverwriteDuringSettle(t *testing.T) {
	prev := reactionSettle
	reactionSettle = 50 * time.Millisecond
	t.Cleanup(func() { reactionSettle = prev })

	api := newAPIMock(t)
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)

	delivered := make(chan string, 4)
	handler := ch.makeHandler(func(_ context.Context, msg channel.Message) (string, error) {
		delivered <- msg.Text
		return "ok", nil
	})

	handler(context.Background(), b, &models.Update{
		MessageReaction: &models.MessageReactionUpdated{
			Chat:        models.Chat{ID: 1},
			MessageID:   5,
			User:        &models.User{ID: 42},
			NewReaction: []models.ReactionType{emojiReaction("❤")},
		},
	})
	handler(context.Background(), b, &models.Update{
		MessageReaction: &models.MessageReactionUpdated{
			Chat:        models.Chat{ID: 1},
			MessageID:   5,
			User:        &models.User{ID: 42},
			OldReaction: []models.ReactionType{emojiReaction("❤")},
			NewReaction: []models.ReactionType{emojiReaction("👍")},
		},
	})

	var got string
	select {
	case got = <-delivered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out")
	}
	select {
	case extra := <-delivered:
		t.Fatalf("unexpected second delivery: %q", extra)
	case <-time.After(100 * time.Millisecond):
	}
	if got != "[reaction] 👍 on: (unknown message)" {
		t.Fatalf("got %q", got)
	}
	waitSendMessage(t, api, 1)
}

func waitSendMessage(t *testing.T, api *apiMock, want int) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for api.count("sendMessage") < want {
		select {
		case <-deadline:
			t.Fatalf("sendMessage calls=%d want %d", api.count("sendMessage"), want)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestMakeHandler_ReactionClearCancelsSettle(t *testing.T) {
	prev := reactionSettle
	reactionSettle = 40 * time.Millisecond
	t.Cleanup(func() { reactionSettle = prev })

	api := newAPIMock(t)
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)
	called := 0
	handler := ch.makeHandler(func(context.Context, channel.Message) (string, error) {
		called++
		return "x", nil
	})

	handler(context.Background(), b, &models.Update{
		MessageReaction: &models.MessageReactionUpdated{
			Chat:        models.Chat{ID: 1},
			MessageID:   1,
			User:        &models.User{ID: 42},
			NewReaction: []models.ReactionType{emojiReaction("👍")},
		},
	})
	handler(context.Background(), b, &models.Update{
		MessageReaction: &models.MessageReactionUpdated{
			Chat:        models.Chat{ID: 1},
			MessageID:   1,
			User:        &models.User{ID: 42},
			OldReaction: []models.ReactionType{emojiReaction("👍")},
			NewReaction: []models.ReactionType{},
		},
	})
	time.Sleep(100 * time.Millisecond)
	if called != 0 {
		t.Fatalf("called=%d, clear should cancel", called)
	}
	if api.count("sendMessage") != 0 {
		t.Fatal("unexpected sends")
	}
}

func TestMakeHandler_ReactionIgnoresBot(t *testing.T) {
	prev := reactionSettle
	reactionSettle = 20 * time.Millisecond
	t.Cleanup(func() { reactionSettle = prev })

	ch := testChannel(t)
	ch.botID = 99
	b := testBot(t, newAPIMock(t).srv.URL)
	called := 0
	handler := ch.makeHandler(func(context.Context, channel.Message) (string, error) {
		called++
		return "x", nil
	})
	handler(context.Background(), b, &models.Update{
		MessageReaction: &models.MessageReactionUpdated{
			Chat:        models.Chat{ID: 1},
			MessageID:   1,
			User:        &models.User{ID: 99, IsBot: true},
			NewReaction: []models.ReactionType{emojiReaction("👍")},
		},
	})
	time.Sleep(60 * time.Millisecond)
	if called != 0 {
		t.Fatalf("called=%d", called)
	}
}
