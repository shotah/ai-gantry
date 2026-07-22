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
	next, again, err := AdvanceNext(job.Kind, job.Expr, job.Timezone, now)
	if err != nil {
		again = false
	}
	enabled := 1
	nextStr := now.Format(time.RFC3339Nano)
	if again {
		nextStr = next.Format(time.RFC3339Nano)
	} else {
		enabled = 0
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE cron_job SET
			running = 0,
			enabled = ?,
			next_run_at = ?,
			last_run_at = ?,
			last_error = ?,
			updated_at = ?
		WHERE id = ?`,
		enabled, nextStr, now.Format(time.RFC3339Nano), errText, now.Format(time.RFC3339Nano), job.ID)
	return err
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
