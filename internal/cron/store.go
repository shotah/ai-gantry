package cron

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// Job is one scheduled turn.
type Job struct {
	ID        int64
	Prompt    string
	Kind      string
	Expr      string
	Timezone  string
	NextRunAt time.Time
	SessionID string
	UserID    string
	ChatID    string
	ThreadID  int
	Enabled   bool
	Running   bool
	CreatedAt time.Time
	UpdatedAt time.Time
	LastRunAt *time.Time
	LastError string
}

// Store persists cron jobs in gantry.db.
type Store struct {
	db      *sql.DB
	maxJobs int
}

// OpenDB attaches cron schema to an existing DB handle.
func OpenDB(db *sql.DB, maxJobs int) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("cron: nil db")
	}
	if maxJobs < 1 {
		maxJobs = 50
	}
	s := &Store{db: db, maxJobs: maxJobs}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS cron_job (
			id          INTEGER PRIMARY KEY,
			prompt      TEXT NOT NULL,
			kind        TEXT NOT NULL,
			expr        TEXT NOT NULL,
			timezone    TEXT NOT NULL,
			next_run_at TEXT NOT NULL,
			session_id  TEXT NOT NULL,
			user_id     TEXT NOT NULL,
			chat_id     TEXT NOT NULL DEFAULT '',
			thread_id   INTEGER NOT NULL DEFAULT 0,
			enabled     INTEGER NOT NULL DEFAULT 1,
			running     INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			last_run_at TEXT,
			last_error  TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cron_due
			ON cron_job(enabled, running, next_run_at)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("cron: migrate: %w", err)
		}
	}
	return nil
}

// MaxJobs returns the configured cap.
func (s *Store) MaxJobs() int { return s.maxJobs }

// ActiveCount returns enabled jobs.
func (s *Store) ActiveCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cron_job WHERE enabled = 1`).Scan(&n)
	return n, err
}

// Schedule inserts a job from a parsed schedule + delivery binding.
func (s *Store) Schedule(ctx context.Context, prompt string, p Parsed, delivery Delivery) (Job, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return Job{}, fmt.Errorf("cron: prompt is required")
	}
	if delivery.SessionID == "" {
		return Job{}, fmt.Errorf("cron: delivery session_id is required")
	}
	n, err := s.ActiveCount(ctx)
	if err != nil {
		return Job{}, err
	}
	if n >= s.maxJobs {
		return Job{}, fmt.Errorf("cron: max active jobs (%d) reached", s.maxJobs)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO cron_job (
			prompt, kind, expr, timezone, next_run_at,
			session_id, user_id, chat_id, thread_id,
			enabled, running, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, ?, ?)`,
		prompt, p.Kind, p.Expr, p.Timezone, p.NextRun.UTC().Format(time.RFC3339Nano),
		delivery.SessionID, delivery.UserID, delivery.ChatID, delivery.ThreadID,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Job{}, fmt.Errorf("cron: insert: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.Get(ctx, id)
}

// List returns enabled jobs (and optionally disabled) newest first.
func (s *Store) List(ctx context.Context, includeDisabled bool) ([]Job, error) {
	q := `
		SELECT id, prompt, kind, expr, timezone, next_run_at,
		       session_id, user_id, chat_id, thread_id,
		       enabled, running, created_at, updated_at, last_run_at, last_error
		FROM cron_job`
	if !includeDisabled {
		q += ` WHERE enabled = 1`
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanJobs(rows)
}

// Cancel disables a job by id.
func (s *Store) Cancel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cron: job %d not found", id)
	}
	return nil
}

// Due returns enabled, non-running jobs with next_run_at <= now.
func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]Job, error) {
	if limit < 1 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, prompt, kind, expr, timezone, next_run_at,
		       session_id, user_id, chat_id, thread_id,
		       enabled, running, created_at, updated_at, last_run_at, last_error
		FROM cron_job
		WHERE enabled = 1 AND running = 0 AND next_run_at <= ?
		ORDER BY next_run_at ASC
		LIMIT ?`, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanJobs(rows)
}

// Claim marks a job running if it is still due and idle.
func (s *Store) Claim(ctx context.Context, id int64, now time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET running = 1, updated_at = ?
		WHERE id = ? AND enabled = 1 AND running = 0 AND next_run_at <= ?`,
		now.UTC().Format(time.RFC3339Nano), id, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Finish clears running and either disables (once) or advances next_run.
func (s *Store) Finish(ctx context.Context, job Job, runErr error) error {
	now := time.Now().UTC()
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
		if len(errText) > 500 {
			errText = errText[:500]
		}
	}
	next, newExpr, again, err := AdvanceNext(job.Kind, job.Expr, job.Timezone, now)
	if err != nil {
		again = false
	}
	enabled := 1
	nextStr := now.Format(time.RFC3339Nano)
	expr := job.Expr
	if again {
		nextStr = next.Format(time.RFC3339Nano)
		if newExpr != "" {
			expr = newExpr
		}
	} else {
		enabled = 0
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE cron_job SET
			running = 0,
			enabled = ?,
			expr = ?,
			next_run_at = ?,
			last_run_at = ?,
			last_error = ?,
			updated_at = ?
		WHERE id = ?`,
		enabled, expr, nextStr, now.Format(time.RFC3339Nano), errText, now.Format(time.RFC3339Nano), job.ID)
	return err
}

// Defer clears running and moves next_run_at forward without finishing the job.
func (s *Store) Defer(ctx context.Context, id int64, until time.Time, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET
			running = 0,
			next_run_at = ?,
			last_error = ?,
			updated_at = ?
		WHERE id = ?`,
		until.UTC().Format(time.RFC3339Nano), reason, now.Format(time.RFC3339Nano), id)
	return err
}

// FindSpark returns the enabled spark *planner* job for a session, if any.
func (s *Store) FindSpark(ctx context.Context, sessionID string) (Job, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, prompt, kind, expr, timezone, next_run_at,
		       session_id, user_id, chat_id, thread_id,
		       enabled, running, created_at, updated_at, last_run_at, last_error
		FROM cron_job
		WHERE enabled = 1 AND kind = ? AND session_id = ?
		ORDER BY id DESC LIMIT 1`, KindSpark, sessionID)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}

// CancelSparkPings disables pending spark_ping jobs for a session.
func (s *Store) CancelSparkPings(ctx context.Context, sessionID string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind = ? AND session_id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), KindSparkPing, sessionID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ScheduleSparkPings inserts one-shot spark_ping jobs at the given times.
func (s *Store) ScheduleSparkPings(ctx context.Context, prompt string, delivery Delivery, tz string, times []time.Time) (int, error) {
	n := 0
	for _, t := range times {
		if !t.After(time.Now().UTC().Add(-time.Second)) {
			continue
		}
		if _, err := s.Schedule(ctx, prompt, SparkPingParsed(t, tz), delivery); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// EnsureSpark creates/refreshes the daily planner and seeds today's ping jobs.
// The planner's next_run is always tomorrow's window start so boot-seeding today
// is not overwritten when the planner wakes at today's start.
func (s *Store) EnsureSpark(ctx context.Context, prompt string, template Parsed, delivery Delivery) (Job, bool, error) {
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
	// Seed remaining day now; planner wakes tomorrow (not today's start).
	template.Kind = KindSpark
	template.Expr = FormatSparkExpr(spec)
	template.NextRun = startToday.Add(24 * time.Hour).UTC()
	template.Timezone = loc.String()

	existing, ok, err := s.FindSpark(ctx, delivery.SessionID)
	if err != nil {
		return Job{}, false, err
	}
	created := false
	if ok {
		if existing.Prompt == prompt && existing.Expr == template.Expr {
			pending, err := s.countSparkPings(ctx, delivery.SessionID)
			if err != nil {
				return Job{}, false, err
			}
			if pending > 0 {
				return existing, false, nil
			}
		} else {
			if err := s.Cancel(ctx, existing.ID); err != nil {
				return Job{}, false, err
			}
			_, _ = s.CancelSparkPings(ctx, delivery.SessionID)
			ok = false
		}
	}
	var job Job
	if !ok {
		job, err = s.Schedule(ctx, prompt, template, delivery)
		if err != nil {
			return Job{}, false, err
		}
		created = true
	} else {
		job = existing
		if err := s.setNextRun(ctx, job.ID, template.NextRun); err != nil {
			return Job{}, false, err
		}
		job.NextRunAt = template.NextRun
	}

	_, _ = s.CancelSparkPings(ctx, delivery.SessionID)
	_, times, err := PlanSparkDayTimes(spec, loc, time.Now())
	if err != nil {
		return job, created, err
	}
	if _, err := s.ScheduleSparkPings(ctx, prompt, delivery, loc.String(), times); err != nil {
		return job, created, err
	}
	return job, created, nil
}

func (s *Store) setNextRun(ctx context.Context, id int64, next time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET next_run_at = ?, updated_at = ? WHERE id = ?`,
		next.UTC().Format(time.RFC3339Nano), now, id)
	return err
}

func (s *Store) countSparkPings(ctx context.Context, sessionID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM cron_job
		WHERE enabled = 1 AND kind = ? AND session_id = ?`, KindSparkPing, sessionID).Scan(&n)
	return n, err
}

// Get loads one job.
func (s *Store) Get(ctx context.Context, id int64) (Job, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, prompt, kind, expr, timezone, next_run_at,
		       session_id, user_id, chat_id, thread_id,
		       enabled, running, created_at, updated_at, last_run_at, last_error
		FROM cron_job WHERE id = ?`, id)
	return scanJob(row)
}

type scannable interface {
	Scan(dest ...any) error
}

func scanJob(row scannable) (Job, error) {
	var j Job
	var next, created, updated string
	var last sql.NullString
	var enabled, running int
	if err := row.Scan(
		&j.ID, &j.Prompt, &j.Kind, &j.Expr, &j.Timezone, &next,
		&j.SessionID, &j.UserID, &j.ChatID, &j.ThreadID,
		&enabled, &running, &created, &updated, &last, &j.LastError,
	); err != nil {
		return Job{}, err
	}
	j.Enabled = enabled != 0
	j.Running = running != 0
	j.NextRunAt, _ = time.Parse(time.RFC3339Nano, next)
	j.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if last.Valid {
		t, err := time.Parse(time.RFC3339Nano, last.String)
		if err == nil {
			j.LastRunAt = &t
		}
	}
	return j, nil
}

func scanJobs(rows *sql.Rows) ([]Job, error) {
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
