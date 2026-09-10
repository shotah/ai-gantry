package cron

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ScheduleFollowUp inserts one KindFollowUp job at at.
func (s *Store) ScheduleFollowUp(ctx context.Context, delivery Delivery, tz string, at time.Time) (Job, error) {
	if tz == "" {
		tz = "UTC"
	}
	return s.Schedule(ctx, FollowUpPrompt, FollowUpParsed(at, tz), delivery)
}

// CancelFollowUps disables pending follow-up jobs for a session.
func (s *Store) CancelFollowUps(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind = ? AND session_id = ?`,
		formatCronTime(time.Now().UTC()), KindFollowUp, sessionID)
	if err != nil {
		return 0, fmt.Errorf("cron: cancel followups: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
