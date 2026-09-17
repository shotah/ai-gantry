package agent

import (
	"context"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
)

// triageReaction records a human's plain acknowledgment without a model
// call. Their 👍 on an agent message is not a question: unless the agent is
// waiting on them (then 👍 is yes and 👎 is no — the model must act) or the
// set holds a negative or unknown emoji, the pair goes to history exactly as
// the model would have written it — the [reaction] line and [silent] — and
// the standing prompt is not billed. handled is true when the turn is done.
func (a *Agent) triageReaction(ctx context.Context, msg channel.Message, text string) (handled bool, err error) {
	start := time.Now()
	emojis, _, ok := channel.ParseReaction(text)
	if !ok || !channel.AllPositive(emojis) {
		return false, nil
	}
	if st, ok := a.talkState(ctx, msg.SessionID); ok && st.WaitingForReply {
		return false, nil
	}
	unlock := a.lockSession(msg.SessionID)
	defer unlock()
	if err := a.sessions.Append(ctx, msg.SessionID,
		session.Message{Role: session.RoleUser, Content: messageStoreText(msg)},
		session.Message{Role: session.RoleAssistant, Content: cron.SilentToken},
	); err != nil {
		return false, err
	}
	userID := strings.TrimSpace(msg.UserID)
	if userID == "" {
		userID = strings.TrimSpace(msg.ChatID)
	}
	a.log.Info("reaction silent skip",
		"session_id", msg.SessionID,
		"user_id", userID,
		"emojis", strings.Join(emojis, " "),
	)
	// Still a turn to the dashboard (gantree-contract.md: source always set),
	// at zero cost — the point of the triage is that this line reads 0.
	totalMS := time.Since(start).Milliseconds()
	a.log.Info("turn perf",
		"source", sourceReaction,
		"session_id", msg.SessionID,
		"outcome", "silent",
		"iterations", 0,
		"tool_calls", 0,
		"max_batch", 0,
		"recoveries", 0,
		"tools_per_inv", 0.0,
		"prompt_est_tokens", 0,
		"gen_est_tokens", 0,
		"model_ms", int64(0),
		"tool_ms", int64(0),
		"total_ms", totalMS,
		"duration_ms", totalMS,
		"hydration_est_tokens", 0,
		"user_id", userID,
		"model", a.model,
	)
	return true, nil
}

// talkState reads the session's wait flags when the store keeps them.
func (a *Agent) talkState(ctx context.Context, sessionID string) (session.TalkState, bool) {
	type reader interface {
		TalkState(context.Context, string) (session.TalkState, error)
	}
	r, ok := a.sessions.(reader)
	if !ok {
		return session.TalkState{}, false
	}
	st, err := r.TalkState(ctx, sessionID)
	if err != nil {
		a.log.Warn("talk state load failed", "err", err)
		return session.TalkState{}, false
	}
	return st, true
}

// placeReaction hands the reply's [react …] emoji to the mouth. A mouth that
// can put an emoji on the human's message attached a ReactionSink; it gets
// the emoji and the text stays as written (often empty — the reaction is the
// reply). No sink, a spoken turn (nothing to see in a car), or an emoji off
// the palette: the emoji rides as text when there is none, so an
// acknowledgment never becomes silence, and is dropped when there is.
func placeReaction(ctx context.Context, msg channel.Message, emoji, text string) string {
	sink := channel.ReactionSinkFrom(ctx)
	if sink != nil && msg.Input != inputSpoken && channel.InPalette(emoji) {
		sink.Set(emoji)
		return text
	}
	if strings.TrimSpace(text) == "" {
		return emoji
	}
	return text
}
