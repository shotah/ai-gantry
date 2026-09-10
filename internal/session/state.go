package session

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Last-speaker values on TalkState.
const (
	SpeakerUser  = "user"
	SpeakerAgent = "agent"
)

// TalkState is per-session conversation wait / last-speaker (not operator prefs).
type TalkState struct {
	LastSpeaker     string
	WaitingForReply bool
	WaitNudges      int
	WaitSetAt       time.Time
}

// Footer is the volatile prompt line so the model sees who spoke and any open wait.
func (s TalkState) Footer() string {
	if s.LastSpeaker == "" && !s.WaitingForReply && s.WaitNudges == 0 {
		return ""
	}
	speaker := s.LastSpeaker
	if speaker == "" {
		speaker = "none"
	}
	wait := "false"
	if s.WaitingForReply {
		wait = "true"
	}
	return fmt.Sprintf("[conversation] last_speaker=%s waiting_for_reply=%s wait_nudges=%d",
		speaker, wait, s.WaitNudges)
}

// TalkState loads conversation wait flags. Missing session is a zero value.
func (s *Store) TalkState(ctx context.Context, sessionID string) (TalkState, error) {
	if s == nil {
		return TalkState{}, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return TalkState{}, nil
	}
	var st TalkState
	var waiting int
	var setAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT last_speaker, waiting_for_reply, wait_nudges, wait_set_at
		FROM session WHERE id = ?`, sessionID).Scan(
		&st.LastSpeaker, &waiting, &st.WaitNudges, &setAt)
	if err == sql.ErrNoRows {
		return TalkState{}, nil
	}
	if err != nil {
		return TalkState{}, fmt.Errorf("session: talk state: %w", err)
	}
	st.WaitingForReply = waiting != 0
	if strings.TrimSpace(setAt) != "" {
		if t, err := time.Parse(time.RFC3339Nano, setAt); err == nil {
			st.WaitSetAt = t
		}
	}
	return st, nil
}

// ArmWait marks the agent as waiting for a human reply and resets nudge count.
func (s *Store) ArmWait(ctx context.Context, sessionID string) error {
	if s == nil {
		return fmt.Errorf("session: nil store")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session: empty session_id")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session (id, summary, updated_at, last_speaker, waiting_for_reply, wait_nudges, wait_set_at)
		VALUES (?, '', ?, ?, 1, 0, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_speaker = excluded.last_speaker,
			waiting_for_reply = 1,
			wait_nudges = 0,
			wait_set_at = excluded.wait_set_at,
			updated_at = excluded.updated_at`,
		sessionID, now, SpeakerAgent, now)
	if err != nil {
		return fmt.Errorf("session: arm wait: %w", err)
	}
	return nil
}

// ClearWait drops the follow-up campaign. last_speaker is left as-is.
func (s *Store) ClearWait(ctx context.Context, sessionID string) error {
	if s == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		UPDATE session SET waiting_for_reply = 0, wait_nudges = 0, wait_set_at = '', updated_at = ?
		WHERE id = ?`, now, sessionID)
	if err != nil {
		return fmt.Errorf("session: clear wait: %w", err)
	}
	return nil
}

// BumpWaitNudge increments wait_nudges when still waiting. No-op if not waiting.
func (s *Store) BumpWaitNudge(ctx context.Context, sessionID string) (TalkState, error) {
	if s == nil {
		return TalkState{}, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return TalkState{}, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		UPDATE session SET wait_nudges = wait_nudges + 1, updated_at = ?
		WHERE id = ? AND waiting_for_reply = 1`, now, sessionID)
	if err != nil {
		return TalkState{}, fmt.Errorf("session: bump wait: %w", err)
	}
	return s.TalkState(ctx, sessionID)
}
