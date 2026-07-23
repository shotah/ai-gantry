// Package discord implements a Discord Gateway channel (DMs first).
//
// Auth model is allowlist-only (DISCORD_ALLOWED_USERS) — no pairing flow.
// Empty allowlist is rejected at config validation time.
// Connection is outbound WebSocket only (no inbound ports).
package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/shotah/ai-gantry/internal/channel"
)

const (
	typingInterval = 5 * time.Second
	chunkPause     = 100 * time.Millisecond
)

// Config configures the Discord channel.
type Config struct {
	Token         string
	AllowedUsers  []string // Discord snowflake user IDs
	Logger        *slog.Logger
	StreamReplies bool // reserved; v1 buffers full replies (edits are phase 2)
}

// sessionFactory builds a discordgo session (overridable in tests).
type sessionFactory func(token string) (session, error)

// session is the discordgo surface we use (narrow for tests).
type session interface {
	AddHandler(handler interface{}) func()
	Open() error
	Close() error
	ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelTyping(channelID string, options ...discordgo.RequestOption) error
	UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	BotUserID() string
}

type discordSession struct {
	*discordgo.Session
}

func (d *discordSession) BotUserID() string {
	if d.State != nil && d.State.User != nil {
		return d.State.User.ID
	}
	return ""
}

func defaultSessionFactory(token string) (session, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	dg.Identify.Intents = discordgo.IntentDirectMessages | discordgo.IntentMessageContent
	return &discordSession{Session: dg}, nil
}

// Channel connects to the Discord Gateway and fans DM messages into a Handler.
type Channel struct {
	token         string
	allowed       map[string]struct{}
	log           *slog.Logger
	newSession    sessionFactory
	chunkMax      int
	streamReplies bool

	mu   sync.Mutex
	sess session // set while Run is active; used by Push
}

// New builds a Discord channel. Token and a non-empty allowlist are required.
func New(cfg Config) (*Channel, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("discord: token is required")
	}
	allowed := make(map[string]struct{})
	for _, id := range cfg.AllowedUsers {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		allowed[id] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("discord: allowlist is empty (pairing is not supported; set DISCORD_ALLOWED_USERS)")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Channel{
		token:         strings.TrimSpace(cfg.Token),
		allowed:       allowed,
		log:           log,
		newSession:    defaultSessionFactory,
		chunkMax:      discordMaxMessageRunes,
		streamReplies: cfg.StreamReplies,
	}, nil
}

// Run opens the Gateway until ctx is cancelled.
func (c *Channel) Run(ctx context.Context, handle channel.Handler) error {
	s, err := c.newSession(c.token)
	if err != nil {
		return fmt.Errorf("discord: create session: %w", err)
	}
	c.mu.Lock()
	c.sess = s
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.sess = nil
		c.mu.Unlock()
		_ = s.Close()
	}()

	s.AddHandler(func(_ *discordgo.Session, ready *discordgo.Ready) {
		c.log.Info("discord connected",
			"bot_id", ready.User.ID,
			"username", ready.User.Username,
			"allowlist_users", len(c.allowed),
		)
	})
	s.AddHandler(c.makeMessageHandler(ctx, handle))

	if err := s.Open(); err != nil {
		return fmt.Errorf("discord: open gateway: %w", err)
	}

	<-ctx.Done()
	return nil
}

func (c *Channel) makeMessageHandler(ctx context.Context, handle channel.Handler) interface{} {
	return func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author == nil || m.Author.Bot {
			return
		}
		// DMs only for v1 (guild mentions later).
		if m.GuildID != "" {
			return
		}
		userID := m.Author.ID
		if !c.isAllowed(userID) {
			c.log.Info("discord ignore unauthorized user",
				"user_id", userID,
				"username", m.Author.Username,
			)
			return
		}
		text := strings.TrimSpace(m.Content)
		if text == "" {
			return // stickers / empty; attachments are phase 2
		}

		c.mu.Lock()
		s := c.sess
		c.mu.Unlock()
		if s == nil {
			return
		}
		if botID := s.BotUserID(); botID != "" && userID == botID {
			return
		}

		sessionID := sessionKey(m.ChannelID, userID)
		stopTyping := c.startTyping(ctx, s, m.ChannelID)
		defer stopTyping()

		// StreamReplies reserved for phase 2 (message edits).
		_ = c.streamReplies

		reply, err := handle(ctx, channel.Message{
			SessionID: sessionID,
			UserID:    userID,
			ChatID:    m.ChannelID,
			Text:      text,
		})
		if err != nil {
			c.log.Error("discord handler error", "err", err, "session_id", sessionID)
			_, _ = s.ChannelMessageSend(m.ChannelID, "sorry — something went wrong handling that message")
			return
		}
		if reply == "" {
			return
		}
		if err := c.sendReply(ctx, s, m.ChannelID, reply); err != nil {
			c.log.Error("discord send failed", "err", err, "session_id", sessionID)
		}
	}
}

func (c *Channel) isAllowed(userID string) bool {
	_, ok := c.allowed[userID]
	return ok
}

func sessionKey(channelID, userID string) string {
	return fmt.Sprintf("discord:%s:%s", channelID, userID)
}

func (c *Channel) startTyping(ctx context.Context, s session, channelID string) func() {
	done := make(chan struct{})
	go func() {
		send := func() {
			_ = s.ChannelTyping(channelID)
		}
		send()
		t := time.NewTicker(typingInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				send()
			}
		}
	}()
	return func() { close(done) }
}

// Push sends a proactive DM (cron). Allowlist enforced.
func (c *Channel) Push(ctx context.Context, msg channel.Outbound) error {
	if msg.UserID != "" && !c.isAllowed(msg.UserID) {
		return fmt.Errorf("discord: push denied for user %s", msg.UserID)
	}

	c.mu.Lock()
	s := c.sess
	c.mu.Unlock()

	var err error
	if s == nil {
		s, err = c.newSession(c.token)
		if err != nil {
			return fmt.Errorf("discord: push session: %w", err)
		}
		if err := s.Open(); err != nil {
			_ = s.Close()
			return fmt.Errorf("discord: push open: %w", err)
		}
		defer func() { _ = s.Close() }()
	}

	channelID, err := resolveChannelID(s, msg)
	if err != nil {
		return err
	}
	return c.sendReply(ctx, s, channelID, msg.Text)
}

func resolveChannelID(s session, msg channel.Outbound) (string, error) {
	if msg.ChatID != "" {
		return msg.ChatID, nil
	}
	// session: discord:<dmChannelID>:<userID>
	parts := strings.Split(msg.SessionID, ":")
	if len(parts) >= 3 && parts[0] == "discord" {
		return parts[1], nil
	}
	if msg.UserID != "" {
		ch, err := s.UserChannelCreate(msg.UserID)
		if err != nil {
			return "", fmt.Errorf("discord: open dm: %w", err)
		}
		return ch.ID, nil
	}
	return "", fmt.Errorf("discord: missing chat/user id for push")
}

func (c *Channel) sendReply(ctx context.Context, s session, channelID, text string) error {
	parts := splitMessage(text, c.chunkMax)
	for i, part := range parts {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(chunkPause):
			}
		}
		if _, err := s.ChannelMessageSend(channelID, part); err != nil {
			return err
		}
	}
	return nil
}
