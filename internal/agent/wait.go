package agent

import (
	"context"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
)

// WaitControl arms follow-up pokes when the model asks and the human ghosts.
type WaitControl interface {
	OnUserTurn(ctx context.Context, sessionID string) error
	AfterReply(ctx context.Context, delivery cron.Delivery, userText, reply string) error
}

func (a *Agent) talkFooter(ctx context.Context, sessionID string) string {
	type reader interface {
		TalkState(context.Context, string) (session.TalkState, error)
	}
	r, ok := a.sessions.(reader)
	if !ok {
		return ""
	}
	st, err := r.TalkState(ctx, sessionID)
	if err != nil {
		a.log.Warn("talk state load failed", "err", err)
		return ""
	}
	return st.Footer()
}
