package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/shotah/ai-gantry/internal/channel"
)

// How long to wait after the last reaction change before piping to the agent.
// Gives the user time to heart → thumbs-up → smile without multiple turns.
var reactionSettle = 3 * time.Second

type settleKey struct {
	userID int64
	chatID int64
	msgID  int
}

type pendingReaction struct {
	gen      int
	timer    *time.Timer
	emojis   []string
	target   string
	threadID int
	userID   int64
	chatID   int64
}

type reactionSettler struct {
	mu      sync.Mutex
	pending map[settleKey]*pendingReaction
}

func newReactionSettler() *reactionSettler {
	return &reactionSettler{pending: make(map[settleKey]*pendingReaction)}
}

func reactionLabel(rt models.ReactionType) string {
	switch rt.Type {
	case models.ReactionTypeTypeEmoji:
		if rt.ReactionTypeEmoji != nil {
			return strings.TrimSpace(rt.ReactionTypeEmoji.Emoji)
		}
	case models.ReactionTypeTypeCustomEmoji:
		if rt.ReactionTypeCustomEmoji != nil {
			id := strings.TrimSpace(rt.ReactionTypeCustomEmoji.CustomEmojiID)
			if id != "" {
				return "[custom:" + id + "]"
			}
		}
	case models.ReactionTypeTypePaid:
		return "[paid]"
	}
	return ""
}

// currentReactionLabels returns the emoji set currently on the message.
func currentReactionLabels(list []models.ReactionType) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, rt := range list {
		label := reactionLabel(rt)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func formatReactionText(emojis []string, target string) string {
	return fmt.Sprintf("[reaction] %s on: %s", strings.Join(emojis, " "), clipReactionTarget(target))
}

// scheduleReaction waits for reactionSettle of quiet time, overwriting with the
// latest emoji set if the user keeps changing their mind.
func (c *Channel) scheduleReaction(ctx context.Context, b *bot.Bot, handle channel.Handler, key settleKey, emojis []string, target string, threadID int) {
	if c.reactSettle == nil {
		return
	}
	c.reactSettle.mu.Lock()
	defer c.reactSettle.mu.Unlock()

	if len(emojis) == 0 {
		if p, ok := c.reactSettle.pending[key]; ok {
			if p.timer != nil {
				p.timer.Stop()
			}
			delete(c.reactSettle.pending, key)
		}
		return
	}

	p, ok := c.reactSettle.pending[key]
	if !ok {
		p = &pendingReaction{userID: key.userID, chatID: key.chatID}
		c.reactSettle.pending[key] = p
	} else if p.timer != nil {
		p.timer.Stop()
	}
	p.gen++
	gen := p.gen
	p.emojis = append([]string(nil), emojis...)
	p.target = target
	p.threadID = threadID
	p.timer = time.AfterFunc(reactionSettle, func() {
		c.flushReaction(ctx, b, handle, key, gen)
	})
}

func (c *Channel) flushReaction(ctx context.Context, b *bot.Bot, handle channel.Handler, key settleKey, gen int) {
	c.reactSettle.mu.Lock()
	p, ok := c.reactSettle.pending[key]
	if !ok || p.gen != gen {
		c.reactSettle.mu.Unlock()
		return
	}
	delete(c.reactSettle.pending, key)
	emojis := append([]string(nil), p.emojis...)
	target := p.target
	threadID := p.threadID
	userID := p.userID
	chatID := p.chatID
	c.reactSettle.mu.Unlock()

	if len(emojis) == 0 || ctx.Err() != nil {
		return
	}
	c.deliver(ctx, b, handle, channel.Message{
		SessionID: sessionKey(chatID, userID, threadID),
		UserID:    strconv.FormatInt(userID, 10),
		ChatID:    strconv.FormatInt(chatID, 10),
		ThreadID:  threadID,
		Text:      formatReactionText(emojis, target),
	}, chatID, threadID)
}
