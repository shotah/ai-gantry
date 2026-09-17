package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/shotah/ai-gantry/internal/channel"
)

// How long to wait after the last reaction change before piping to the agent.
// Gives the user time to heart → thumbs-up → smile without multiple turns.
var reactionSettle = 3 * time.Second

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

// scheduleReaction waits for reactionSettle of quiet time, overwriting with
// the latest emoji set if the user keeps changing their mind; an empty set
// (reaction cleared) cancels. The settled set is delivered as a
// [reaction] user turn; there is no human message to react back on.
func (c *Channel) scheduleReaction(ctx context.Context, b *bot.Bot, handle channel.Handler, userID, chatID int64, msgID int, emojis []string, target string, threadID int) {
	key := fmt.Sprintf("%d/%d/%d", userID, chatID, msgID)
	c.reactSettle.Schedule(key, emojis, reactionSettle, func(set []string) {
		if ctx.Err() != nil {
			return
		}
		c.deliver(ctx, b, handle, channel.Message{
			SessionID: channel.AgentSession,
			UserID:    strconv.FormatInt(userID, 10),
			ChatID:    strconv.FormatInt(chatID, 10),
			ThreadID:  threadID,
			Text:      channel.FormatReaction(set, target),
		}, chatID, threadID, 0)
	})
}

// setReaction puts the agent's emoji on the human's message. Telegram wants
// the bare code point ("❤", not "❤️"). False when the API refused, so the
// caller can say it in text instead.
func (c *Channel) setReaction(ctx context.Context, b *bot.Bot, chatID int64, msgID int, emoji string) bool {
	emoji = strings.TrimSuffix(strings.TrimSpace(emoji), "\ufe0f")
	ok, err := b.SetMessageReaction(ctx, &bot.SetMessageReactionParams{
		ChatID:    chatID,
		MessageID: msgID,
		Reaction: []models.ReactionType{{
			Type: models.ReactionTypeTypeEmoji,
			ReactionTypeEmoji: &models.ReactionTypeEmoji{
				Type:  models.ReactionTypeTypeEmoji,
				Emoji: emoji,
			},
		}},
	})
	if err != nil || !ok {
		c.log.Warn("telegram reaction refused; saying it in text", "err", err, "emoji", emoji, "message_id", msgID)
		return false
	}
	return true
}
