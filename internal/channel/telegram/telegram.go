// Package telegram implements a Telegram long-poll channel.
//
// Auth model is allowlist-only (TELEGRAM_ALLOWED_USERS) — no pairing flow.
// Empty allowlist is rejected at config validation time.
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/shotah/ai-gantry/internal/channel"
)

const (
	typingInterval = 4 * time.Second
	chunkPause     = 100 * time.Millisecond
)

// Config configures the Telegram channel.
type Config struct {
	Token         string
	AllowedUsers  []int64
	Logger        *slog.Logger
	StreamReplies bool // placeholder + editMessageText while the model streams
}

// Channel long-polls Telegram and fans messages into a channel.Handler.
type Channel struct {
	token         string
	allowed       map[int64]struct{}
	log           *slog.Logger
	newBot        func(token string, opts ...bot.Option) (*bot.Bot, error)
	chunkMax      int
	streamReplies bool
	botID         int64
	outbound      *outboundCache
	reactSettle   *reactionSettler
}

// New builds a Telegram channel. Token and a non-empty allowlist are required.
func New(cfg Config) (*Channel, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("telegram: token is required")
	}
	if len(cfg.AllowedUsers) == 0 {
		return nil, fmt.Errorf("telegram: allowlist is empty (pairing is not supported; set TELEGRAM_ALLOWED_USERS)")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	allowed := make(map[int64]struct{}, len(cfg.AllowedUsers))
	for _, id := range cfg.AllowedUsers {
		allowed[id] = struct{}{}
	}
	return &Channel{
		token:         cfg.Token,
		allowed:       allowed,
		log:           log,
		newBot:        bot.New,
		chunkMax:      telegramMaxMessageRunes,
		streamReplies: cfg.StreamReplies,
		outbound:      newOutboundCache(outboundCacheCap),
		reactSettle:   newReactionSettler(),
	}, nil
}

// Run starts long-polling until ctx is cancelled.
func (c *Channel) Run(ctx context.Context, handle channel.Handler) error {
	b, err := c.newBot(c.token,
		bot.WithDefaultHandler(c.makeHandler(handle)),
		bot.WithWorkers(1), // one-at-a-time; keeps session writes simple
		bot.WithAllowedUpdates(bot.AllowedUpdates{
			models.AllowedUpdateMessage,
			models.AllowedUpdateMessageReaction,
		}),
		bot.WithErrorsHandler(func(err error) {
			c.log.Error("telegram bot error", "err", err)
		}),
	)
	if err != nil {
		return fmt.Errorf("telegram: create bot: %w", err)
	}

	if _, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "new", Description: "Reset conversation session"},
			{Command: "status", Description: "Show uptime, model, history"},
		},
	}); err != nil {
		c.log.Warn("telegram: setMyCommands failed", "err", err)
	}

	me, err := b.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("telegram: getMe: %w", err)
	}
	c.botID = me.ID
	c.log.Info("telegram connected",
		"bot_id", me.ID,
		"username", me.Username,
		"allowlist_users", len(c.allowed),
	)

	b.Start(ctx) // blocks until ctx cancel
	return nil
}

func (c *Channel) makeHandler(handle channel.Handler) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.MessageReaction != nil {
			c.handleReaction(ctx, b, handle, update.MessageReaction)
			return
		}
		if update.Message == nil || update.Message.From == nil {
			return
		}
		msg := update.Message
		userID := msg.From.ID
		if !c.isAllowed(userID) {
			c.log.Info("telegram ignore unauthorized user",
				"user_id", userID,
				"username", msg.From.Username,
			)
			return
		}

		text := strings.TrimSpace(msg.Text)
		if text == "" {
			text = strings.TrimSpace(msg.Caption)
		}
		images, err := inboundImages(ctx, b, msg)
		if err != nil {
			c.log.Error("telegram photo download failed", "err", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          msg.Chat.ID,
				MessageThreadID: msg.MessageThreadID,
				Text:            "sorry — couldn't download that photo",
			})
			return
		}
		if text == "" && len(images) == 0 {
			return // ignore stickers / empty updates
		}

		c.deliver(ctx, b, handle, channel.Message{
			SessionID: sessionKey(msg.Chat.ID, userID, msg.MessageThreadID),
			UserID:    strconv.FormatInt(userID, 10),
			ChatID:    strconv.FormatInt(msg.Chat.ID, 10),
			ThreadID:  msg.MessageThreadID,
			Text:      text,
			Images:    images,
		}, msg.Chat.ID, msg.MessageThreadID)
	}
}

func (c *Channel) handleReaction(ctx context.Context, b *bot.Bot, handle channel.Handler, r *models.MessageReactionUpdated) {
	if r == nil || r.User == nil {
		return
	}
	user := r.User
	if user.IsBot || (c.botID != 0 && user.ID == c.botID) {
		return
	}
	if !c.isAllowed(user.ID) {
		c.log.Info("telegram ignore unauthorized reaction",
			"user_id", user.ID,
			"username", user.Username,
		)
		return
	}

	emojis := currentReactionLabels(r.NewReaction)
	key := settleKey{userID: user.ID, chatID: r.Chat.ID, msgID: r.MessageID}
	target := ""
	threadID := 0
	if entry, ok := c.outbound.lookup(r.Chat.ID, r.MessageID); ok {
		target = entry.text
		threadID = entry.threadID
	}
	// Empty set cancels a pending settle (user cleared the reaction).
	c.scheduleReaction(ctx, b, handle, key, emojis, target, threadID)
}

func (c *Channel) deliver(ctx context.Context, b *bot.Bot, handle channel.Handler, msg channel.Message, chatID int64, threadID int) {
	stopTyping := c.startTyping(ctx, b, chatID, threadID)
	defer stopTyping()

	var stream *editStream
	handleCtx := ctx
	if c.streamReplies {
		stream = newEditStream(b, chatID, threadID, c.chunkMax)
		stream.onSent = func(msgID int, text string) {
			c.outbound.remember(chatID, msgID, threadID, text)
		}
		handleCtx = channel.WithReplyWriter(ctx, stream)
	}

	reply, err := handle(handleCtx, msg)
	if err != nil {
		c.log.Error("telegram handler error", "err", err, "session_id", msg.SessionID)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            "sorry — something went wrong handling that message",
		})
		return
	}
	if stream != nil && stream.Started() {
		urls, rest := channel.ExtractImageURLs(reply)
		if err := stream.Finish(ctx, rest); err != nil {
			c.log.Warn("telegram stream finish failed; falling back to send", "err", err)
			if reply != "" {
				if err := c.sendReply(ctx, b, chatID, threadID, reply, ""); err != nil {
					c.log.Error("telegram send failed", "err", err, "session_id", msg.SessionID)
				}
			}
			return
		}
		for _, u := range urls {
			sent, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
				ChatID:          chatID,
				MessageThreadID: threadID,
				Photo:           &models.InputFileString{Data: u},
			})
			if err != nil {
				c.log.Error("telegram sendPhoto failed", "err", err, "session_id", msg.SessionID)
				continue
			}
			if sent != nil {
				c.outbound.remember(chatID, sent.ID, threadID, "[photo]")
			}
		}
		return
	}
	if reply == "" {
		return
	}
	if err := c.sendReply(ctx, b, chatID, threadID, reply, ""); err != nil {
		c.log.Error("telegram send failed", "err", err, "session_id", msg.SessionID)
	}
}

func (c *Channel) isAllowed(userID int64) bool {
	_, ok := c.allowed[userID]
	return ok
}

func sessionKey(chatID, userID int64, threadID int) string {
	if threadID > 0 {
		return fmt.Sprintf("telegram:%d:%d:%d", chatID, userID, threadID)
	}
	return fmt.Sprintf("telegram:%d:%d", chatID, userID)
}

func (c *Channel) startTyping(ctx context.Context, b *bot.Bot, chatID int64, threadID int) func() {
	done := make(chan struct{})
	go func() {
		send := func() {
			_, _ = b.SendChatAction(ctx, &bot.SendChatActionParams{
				ChatID:          chatID,
				MessageThreadID: threadID,
				Action:          models.ChatActionTyping,
			})
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

// Push sends a proactive message (cron) to the job's chat. Allowlist enforced.
func (c *Channel) Push(ctx context.Context, msg channel.Outbound) error {
	chatID, err := resolveChatID(msg)
	if err != nil {
		return err
	}
	userID, _ := strconv.ParseInt(msg.UserID, 10, 64)
	if userID != 0 && !c.isAllowed(userID) {
		return fmt.Errorf("telegram: push denied for user %d", userID)
	}
	b, err := c.newBot(c.token)
	if err != nil {
		return fmt.Errorf("telegram: push bot: %w", err)
	}
	return c.sendReply(ctx, b, chatID, msg.ThreadID, msg.Text, msg.PhotoURL)
}

func resolveChatID(msg channel.Outbound) (int64, error) {
	if msg.ChatID != "" {
		return strconv.ParseInt(msg.ChatID, 10, 64)
	}
	// session: telegram:<chat>:<user>[:thread]
	parts := strings.Split(msg.SessionID, ":")
	if len(parts) >= 3 && parts[0] == "telegram" {
		return strconv.ParseInt(parts[1], 10, 64)
	}
	return 0, fmt.Errorf("telegram: missing chat id for push")
}

func (c *Channel) sendChunks(ctx context.Context, b *bot.Bot, chatID int64, threadID int, text string) error {
	parts := splitMessage(text, c.chunkMax)
	for i, part := range parts {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(chunkPause):
			}
		}
		var sent *models.Message
		if err := doWith429Retry(ctx, func() error {
			m, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chatID,
				MessageThreadID: threadID,
				Text:            part,
			})
			if err != nil {
				return err
			}
			sent = m
			return nil
		}); err != nil {
			return err
		}
		if sent != nil {
			c.outbound.remember(chatID, sent.ID, threadID, part)
		}
	}
	return nil
}
