package discord

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestNew_RequiresTokenAndAllowlist(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected token error")
	}
	if _, err := New(Config{Token: "x"}); err == nil {
		t.Fatal("expected allowlist error")
	}
	ch, err := New(Config{Token: "x", AllowedUsers: []string{"1", " 2 "}})
	if err != nil {
		t.Fatal(err)
	}
	if !ch.isAllowed("2") || !ch.isAllowed("1") {
		t.Fatal("allowlist")
	}
}

func TestSessionKeyAndResolve(t *testing.T) {
	key := sessionKey("ch99", "user7")
	if key != "discord:ch99:user7" {
		t.Fatalf("key = %q", key)
	}
	id, err := resolveChannelID(nil, channel.Outbound{ChatID: "abc"})
	if err != nil || id != "abc" {
		t.Fatalf("%q %v", id, err)
	}
	id, err = resolveChannelID(nil, channel.Outbound{SessionID: "discord:ch99:user7"})
	if err != nil || id != "ch99" {
		t.Fatalf("%q %v", id, err)
	}
	if _, err := resolveChannelID(nil, channel.Outbound{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestSplitMessage(t *testing.T) {
	parts := splitMessage("hello", 2000)
	if len(parts) != 1 || parts[0] != "hello" {
		t.Fatalf("%v", parts)
	}
	long := ""
	for i := 0; i < 2500; i++ {
		long += "a"
	}
	parts = splitMessage(long, 2000)
	if len(parts) != 2 {
		t.Fatalf("parts=%d", len(parts))
	}
}

type mockSession struct {
	mu       sync.Mutex
	msgs     []string
	edits    []string
	embeds   int
	files    int
	channels []string
	typing   int
	botID    string
	nextID   int
	openErr  error
	handlers []interface{}
}

func (m *mockSession) AddHandler(handler interface{}) func() {
	m.handlers = append(m.handlers, handler)
	return func() {}
}

func (m *mockSession) Open() error { return m.openErr }

func (m *mockSession) Close() error { return nil }

func (m *mockSession) BotUserID() string {
	return m.botID
}

func (m *mockSession) ChannelMessageSend(channelID, content string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	m.channels = append(m.channels, channelID)
	m.msgs = append(m.msgs, content)
	return &discordgo.Message{ID: fmt.Sprintf("m%d", m.nextID), Content: content}, nil
}

func (m *mockSession) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	m.channels = append(m.channels, channelID)
	if data != nil {
		if data.Content != "" {
			m.msgs = append(m.msgs, data.Content)
		}
		m.embeds += len(data.Embeds)
		m.files += len(data.Files)
	}
	return &discordgo.Message{ID: fmt.Sprintf("m%d", m.nextID)}, nil
}

func (m *mockSession) ChannelMessageEdit(channelID, messageID, content string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = append(m.channels, channelID)
	m.edits = append(m.edits, messageID+":"+content)
	return &discordgo.Message{ID: messageID, Content: content}, nil
}

func (m *mockSession) ChannelTyping(string, ...discordgo.RequestOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typing++
	return nil
}

func (m *mockSession) UserChannelCreate(recipientID string, _ ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return &discordgo.Channel{ID: "dm-" + recipientID}, nil
}

func TestChannel_PushAllowlistAndSend(t *testing.T) {
	mock := &mockSession{}
	ch, err := New(Config{Token: "tok", AllowedUsers: []string{"42"}})
	if err != nil {
		t.Fatal(err)
	}
	ch.newSession = func(string) (session, error) { return mock, nil }

	err = ch.Push(context.Background(), channel.Outbound{
		UserID: "42",
		ChatID: "dm-42",
		Text:   "cron hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.msgs) != 1 || mock.msgs[0] != "cron hello" {
		t.Fatalf("msgs=%v", mock.msgs)
	}

	err = ch.Push(context.Background(), channel.Outbound{
		UserID: "999",
		ChatID: "dm-42",
		Text:   "nope",
	})
	if err == nil {
		t.Fatal("expected allowlist deny")
	}
}

func TestChannel_RunHandlesDM(t *testing.T) {
	mock := &mockSession{botID: "bot1"}
	ch, err := New(Config{Token: "tok", AllowedUsers: []string{"42"}})
	if err != nil {
		t.Fatal(err)
	}
	ch.newSession = func(string) (session, error) { return mock, nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handled := make(chan channel.Message, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ch.Run(ctx, func(_ context.Context, msg channel.Message) (string, error) {
			handled <- msg
			return "pong", nil
		})
	}()

	// Wait for handlers to register.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(mock.handlers) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(mock.handlers) < 2 {
		t.Fatal("handlers not registered")
	}

	var msgHandler func(*discordgo.Session, *discordgo.MessageCreate)
	for _, h := range mock.handlers {
		if fn, ok := h.(func(*discordgo.Session, *discordgo.MessageCreate)); ok {
			msgHandler = fn
		}
	}
	if msgHandler == nil {
		t.Fatal("message handler missing")
	}

	msgHandler(nil, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "dmchan",
			Content:   "/status",
			Author:    &discordgo.User{ID: "42", Username: "alice"},
		},
	})

	select {
	case got := <-handled:
		if got.UserID != "42" || got.ChatID != "dmchan" || got.Text != "/status" {
			t.Fatalf("%+v", got)
		}
		if got.SessionID != "discord:dmchan:42" {
			t.Fatalf("session=%q", got.SessionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called")
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mock.mu.Lock()
		n := len(mock.msgs)
		mock.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mock.mu.Lock()
	if len(mock.msgs) < 1 || mock.msgs[0] != "pong" {
		t.Fatalf("msgs=%v", mock.msgs)
	}
	mock.mu.Unlock()

	// Guild messages ignored.
	msgHandler(nil, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			GuildID:   "guild1",
			ChannelID: "c1",
			Content:   "hi",
			Author:    &discordgo.User{ID: "42"},
		},
	})

	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit")
	}
}
