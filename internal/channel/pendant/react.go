package pendant

import (
	"context"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// reactionSettle is the quiet time after the last reaction change before
// the set goes to the agent — heart → thumbs-up → smile is one turn.
var reactionSettle = 3 * time.Second

// scheduleReaction takes a human's `react` frame — `id` is one of our reply
// or push ids, `text` the emoji set, empty when cleared — and, once it
// settles, runs a [reaction] turn naming what they reacted to. Unknown id:
// "(unknown message)", still handled. No sink on that turn: there is no
// human message to react back on.
func (c *Channel) scheduleReaction(ctx context.Context, cn conn, handle channel.Handler, sub string, frame inboundFrame) {
	id := strings.TrimSpace(frame.ID)
	emojis := strings.Fields(frame.Text)
	target, _ := c.recent.Lookup(id)
	c.reactSettle.Schedule(sub+"/"+id, emojis, reactionSettle, func(set []string) {
		if ctx.Err() != nil {
			return
		}
		msg := channel.Message{
			SessionID: channel.AgentSession,
			UserID:    sub,
			ChatID:    sub,
			Text:      channel.FormatReaction(set, target),
		}
		if err := c.runTurn(ctx, cn, handle, msg, ""); err != nil {
			c.log.Warn("pendant reaction turn failed", "err", err, "session_id", msg.SessionID)
		}
	})
}
