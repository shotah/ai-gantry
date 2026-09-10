package cron

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/session"
)

// WaitService arms/clears waiting_for_reply and schedules follow-up pokes.
type WaitService struct {
	State *session.Store
	Jobs  *Store
	TZ    string
}

// OnUserTurn clears the wait campaign when the human actually replies.
func (w *WaitService) OnUserTurn(ctx context.Context, sessionID string) error {
	if w == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if w.State != nil {
		if err := w.State.ClearWait(ctx, sessionID); err != nil {
			return err
		}
	}
	if w.Jobs != nil {
		if _, err := w.Jobs.CancelFollowUps(ctx, sessionID); err != nil {
			return err
		}
	}
	return nil
}

// AfterReply arms or drops wait from [wait] / [nowait] in the model reply.
// Silent replies never arm. Follow-up turns cannot re-arm (already waiting).
func (w *WaitService) AfterReply(ctx context.Context, delivery Delivery, userText, reply string) error {
	if w == nil {
		return nil
	}
	if IsSilentReply(reply) {
		return nil
	}
	if HasNoWaitToken(reply) {
		return w.OnUserTurn(ctx, delivery.SessionID)
	}
	if !HasWaitToken(reply) || IsFollowUpTurn(userText) {
		return nil
	}
	if w.State == nil {
		return fmt.Errorf("cron: wait service has no session store")
	}
	if err := w.State.ArmWait(ctx, delivery.SessionID); err != nil {
		return err
	}
	if w.Jobs == nil {
		return nil
	}
	if _, err := w.Jobs.CancelFollowUps(ctx, delivery.SessionID); err != nil {
		return err
	}
	delay, ok := NextFollowUpDelay(0)
	if !ok {
		return nil
	}
	tz := strings.TrimSpace(w.TZ)
	if tz == "" {
		tz = "UTC"
	}
	_, err := w.Jobs.ScheduleFollowUp(ctx, delivery, tz, time.Now().UTC().Add(delay))
	return err
}
