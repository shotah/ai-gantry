package cron

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// PlannerAt returns the session clock override. Empty inherits the operator default.
func (s *Store) PlannerAt(ctx context.Context, sessionID string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `
		SELECT planner_at FROM session_pref WHERE session_id = ?`, sessionID).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v), nil
}

// SetPlannerAt persists /planner for a session. Empty inherits the default; "0" is off.
func (s *Store) SetPlannerAt(ctx context.Context, sessionID, at string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("cron: empty session_id")
	}
	at = strings.TrimSpace(at)
	now := formatCronTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_pref (session_id, examples_enabled, planner_at, updated_at)
		VALUES (?, 1, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			planner_at = excluded.planner_at,
			updated_at = excluded.updated_at`,
		sessionID, at, now)
	return err
}

// FindDailyPlanner returns the enabled daily planner for a session, if any.
func (s *Store) FindDailyPlanner(ctx context.Context, sessionID string) (Job, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+jobColumns+`
		FROM cron_job
		WHERE enabled = 1 AND kind = ? AND session_id = ?
		ORDER BY id DESC LIMIT 1`, KindDailyPlanner, sessionID)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}

// CancelDailyPlanner disables the daily planner for a session.
func (s *Store) CancelDailyPlanner(ctx context.Context, sessionID string) (int64, error) {
	now := formatCronTime(time.Now().UTC())
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind = ? AND session_id = ?`,
		now, KindDailyPlanner, sessionID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// EnsureDailyPlanner keeps one daily planning job. Reboots leave a future
// next_run alone. Moving the clock on a job that already ran today schedules
// tomorrow, so the session does not burn twice.
func (s *Store) EnsureDailyPlanner(ctx context.Context, prompt string, template Parsed, delivery Delivery) (Job, bool, error) {
	loc, err := loadTZ(template.Timezone)
	if err != nil {
		loc = time.UTC
	}
	hour, minute, err := ParsePlannerAt(template.Expr)
	if err != nil {
		return Job{}, false, err
	}
	template.Kind = KindDailyPlanner
	template.Expr = FormatPlannerAt(hour, minute)
	template.Timezone = loc.String()
	if template.NextRun.IsZero() {
		template.NextRun = NextPlannerAt(hour, minute, loc, time.Now(), nil, false)
	}

	existing, ok, err := s.FindDailyPlanner(ctx, delivery.SessionID)
	if err != nil {
		return Job{}, false, err
	}
	if !ok {
		job, err := s.Schedule(ctx, prompt, template, delivery)
		if err != nil {
			return Job{}, false, err
		}
		_ = s.disableExtraPlanners(ctx, delivery.SessionID, job.ID)
		return job, true, nil
	}

	_ = s.disableExtraPlanners(ctx, delivery.SessionID, existing.ID)
	if existing.Expr == template.Expr && existing.Prompt == prompt {
		return existing, false, nil
	}
	if existing.Expr != template.Expr {
		template.NextRun = NextPlannerAt(hour, minute, loc, time.Now(), existing.LastRunAt, existing.Running)
	} else {
		template.NextRun = existing.NextRunAt
	}
	if err := s.rewriteDailyPlanner(ctx, existing.ID, prompt, template); err != nil {
		return Job{}, false, err
	}
	job, err := s.Get(ctx, existing.ID)
	if err != nil {
		return Job{}, false, err
	}
	return job, false, nil
}

func (s *Store) rewriteDailyPlanner(ctx context.Context, id int64, prompt string, p Parsed) error {
	now := formatCronTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET
			kind = ?,
			prompt = ?,
			expr = ?,
			timezone = ?,
			next_run_at = ?,
			updated_at = ?
		WHERE id = ?`,
		KindDailyPlanner, prompt, p.Expr, p.Timezone, formatCronTime(p.NextRun.UTC()), now, id)
	return err
}

func (s *Store) disableExtraPlanners(ctx context.Context, sessionID string, keepID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind = ? AND session_id = ? AND id != ?`,
		formatCronTime(time.Now().UTC()), KindDailyPlanner, sessionID, keepID)
	return err
}
