package cron

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ExamplesEnabled reports whether the session wants proactive examples pings.
// Missing row defaults to enabled (on by default).
func (s *Store) ExamplesEnabled(ctx context.Context, sessionID string) (bool, error) {
	var v int
	err := s.db.QueryRowContext(ctx, `
		SELECT examples_enabled FROM session_pref WHERE session_id = ?`, sessionID).Scan(&v)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

// SparkQty returns the session override. Empty means inherit DefaultSparkQty.
func (s *Store) SparkQty(ctx context.Context, sessionID string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `
		SELECT spark_qty FROM session_pref WHERE session_id = ?`, sessionID).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v), nil
}

// SetSparkQty persists /engagement qty for a session. Empty inherits default; "0" is off.
func (s *Store) SetSparkQty(ctx context.Context, sessionID, qty string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("cron: empty session_id")
	}
	qty = strings.TrimSpace(qty)
	now := formatCronTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_pref (session_id, examples_enabled, spark_qty, updated_at)
		VALUES (?, 1, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			spark_qty = excluded.spark_qty,
			updated_at = excluded.updated_at`,
		sessionID, qty, now)
	return err
}

// SetExamplesEnabled persists /examples on|off for a session.
func (s *Store) SetExamplesEnabled(ctx context.Context, sessionID string, on bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("cron: empty session_id")
	}
	flag := 0
	if on {
		flag = 1
	}
	now := formatCronTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_pref (session_id, examples_enabled, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			examples_enabled = excluded.examples_enabled,
			updated_at = excluded.updated_at`,
		sessionID, flag, now)
	return err
}

// FindExamples returns the enabled examples *planner* job for a session, if any.
func (s *Store) FindExamples(ctx context.Context, sessionID string) (Job, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+jobColumns+`
		FROM cron_job
		WHERE enabled = 1 AND kind = ? AND session_id = ?
		ORDER BY id DESC LIMIT 1`, KindExamples, sessionID)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}

// CancelExamplesPings disables pending examples_ping jobs for a session.
func (s *Store) CancelExamplesPings(ctx context.Context, sessionID string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind = ? AND session_id = ?`,
		formatCronTime(time.Now().UTC()), KindExamplesPing, sessionID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// CancelExamplesPlannerAndPings disables the examples planner and pending pings.
func (s *Store) CancelExamplesPlannerAndPings(ctx context.Context, sessionID string) (int64, error) {
	now := formatCronTime(time.Now().UTC())
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind IN (?, ?) AND session_id = ?`,
		now, KindExamples, KindExamplesPing, sessionID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ScheduleExamplesPings inserts one-shot examples_ping jobs at the given times.
func (s *Store) ScheduleExamplesPings(ctx context.Context, prompt string, delivery Delivery, tz string, times []time.Time) (int, error) {
	n := 0
	for _, t := range times {
		if !t.After(time.Now().UTC().Add(-time.Second)) {
			continue
		}
		if _, err := s.Schedule(ctx, prompt, ExamplesPingParsed(t, tz), delivery); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// EnsureExamples creates/refreshes the daily examples planner and seeds today's
// ping jobs at most once per local day (same non-compounding rules as EnsureSpark).
func (s *Store) EnsureExamples(ctx context.Context, prompt string, template Parsed, delivery Delivery) (Job, bool, error) {
	loc, err := loadTZ(template.Timezone)
	if err != nil {
		loc, err = loadTZ("UTC")
		if err != nil {
			return Job{}, false, err
		}
	}
	spec, err := ParseSparkExpr(template.Expr)
	if err != nil {
		return Job{}, false, err
	}
	now := time.Now().In(loc)
	startToday := windowStart(now, spec.StartHour, loc)
	tomorrowStart := addOneCalendarDay(startToday).UTC()
	template.Kind = KindExamples
	template.Expr = FormatSparkExpr(spec)
	template.NextRun = tomorrowStart
	template.Timezone = loc.String()

	existing, ok, err := s.FindExamples(ctx, delivery.SessionID)
	if err != nil {
		return Job{}, false, err
	}
	if ok && (existing.Prompt != prompt || existing.Expr != template.Expr) {
		if err := s.Cancel(ctx, existing.ID); err != nil {
			return Job{}, false, err
		}
		_, _ = s.CancelExamplesPings(ctx, delivery.SessionID)
		ok = false
	}

	if !ok {
		job, err := s.Schedule(ctx, prompt, template, delivery)
		if err != nil {
			return Job{}, false, err
		}
		_ = s.disableExtraExamplesPlanners(ctx, delivery.SessionID, job.ID)
		if err := s.seedExamplesDay(ctx, prompt, spec, delivery, loc); err != nil {
			return job, true, err
		}
		return job, true, nil
	}

	job := existing
	_ = s.disableExtraExamplesPlanners(ctx, delivery.SessionID, job.ID)

	if _, err := s.CancelStaleExamplesPings(ctx, delivery.SessionID, startToday); err != nil {
		return Job{}, false, err
	}

	if !job.NextRunAt.Before(tomorrowStart) {
		return job, false, nil
	}

	if err := s.setNextRun(ctx, job.ID, template.NextRun); err != nil {
		return Job{}, false, err
	}
	job.NextRunAt = template.NextRun
	if err := s.seedExamplesDay(ctx, prompt, spec, delivery, loc); err != nil {
		return job, false, err
	}
	return job, false, nil
}

func (s *Store) seedExamplesDay(ctx context.Context, prompt string, spec SparkSpec, delivery Delivery, loc *time.Location) error {
	_, _ = s.CancelExamplesPings(ctx, delivery.SessionID)
	_, times, err := PlanSparkDayTimes(spec, loc, time.Now())
	if err != nil {
		return err
	}
	_, err = s.ScheduleExamplesPings(ctx, prompt, delivery, loc.String(), times)
	return err
}

// CancelStaleExamplesPings disables pending examples_ping jobs scheduled before before.
func (s *Store) CancelStaleExamplesPings(ctx context.Context, sessionID string, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind = ? AND session_id = ? AND next_run_at < ?`,
		formatCronTime(time.Now().UTC()), KindExamplesPing, sessionID, formatCronTime(before.UTC()))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *Store) disableExtraExamplesPlanners(ctx context.Context, sessionID string, keepID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind = ? AND session_id = ? AND id != ?`,
		formatCronTime(time.Now().UTC()), KindExamples, sessionID, keepID)
	return err
}
