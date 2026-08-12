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
		`CREATE TABLE IF NOT EXISTS session_pref (
			session_id       TEXT PRIMARY KEY,
			examples_enabled INTEGER NOT NULL DEFAULT 1,
			updated_at       TEXT NOT NULL
		)`,
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
		prompt, p.Kind, p.Expr, p.Timezone, formatCronTime(p.NextRun.UTC()),
		delivery.SessionID, delivery.UserID, delivery.ChatID, delivery.ThreadID,
		formatCronTime(now), formatCronTime(now),
	)
	if err != nil {
		return Job{}, fmt.Errorf("cron: insert: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.Get(ctx, id)
}

// List returns enabled jobs (and optionally disabled) newest first.
func (s *Store) List(ctx context.Context, includeDisabled bool) ([]Job, error) {
	return s.ListSession(ctx, "", includeDisabled)
}

// ListSession returns jobs for sessionID (empty sessionID = all sessions).
func (s *Store) ListSession(ctx context.Context, sessionID string, includeDisabled bool) ([]Job, error) {
	q := `
		SELECT id, prompt, kind, expr, timezone, next_run_at,
		       session_id, user_id, chat_id, thread_id,
		       enabled, running, created_at, updated_at, last_run_at, last_error
		FROM cron_job WHERE 1=1`
	args := []any{}
	if !includeDisabled {
		q += ` AND enabled = 1`
	}
	if sessionID != "" {
		q += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanJobs(rows)
}

// ClearStaleRunning resets running=1 left by a crash/OOM so Due can see jobs again.
// Call once at runner boot.
func (s *Store) ClearStaleRunning(ctx context.Context) (int64, error) {
	now := formatCronTime(time.Now().UTC())
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET running = 0, updated_at = ? WHERE running = 1`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Cancel disables a job by id. If the job is a spark or examples planner,
// pending ping rows for that session are cancelled too.
func (s *Store) Cancel(ctx context.Context, id int64) error {
	job, err := s.Get(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("cron: job %d not found", id)
		}
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ? WHERE id = ?`,
		formatCronTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cron: job %d not found", id)
	}
	switch job.Kind {
	case KindSpark:
		_, _ = s.CancelSparkPings(ctx, job.SessionID)
	case KindExamples:
		_, _ = s.CancelExamplesPings(ctx, job.SessionID)
	}
	return nil
}

// Due returns enabled, non-running jobs with next_run_at <= now.
func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]Job, error) {
	if limit < 1 {
		limit = 10
	}
	// Daily planners first so they cancel pending pings before overdue leftovers Claim.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, prompt, kind, expr, timezone, next_run_at,
		       session_id, user_id, chat_id, thread_id,
		       enabled, running, created_at, updated_at, last_run_at, last_error
		FROM cron_job
		WHERE enabled = 1 AND running = 0 AND next_run_at <= ?
		ORDER BY CASE WHEN kind IN (?, ?) THEN 0 ELSE 1 END, next_run_at ASC
		LIMIT ?`, formatCronTime(now.UTC()), KindSpark, KindExamples, limit)
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
		formatCronTime(now.UTC()), id, formatCronTime(now.UTC()))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Finish clears running and either disables (once) or advances next_run.
// Cancel-safe: only updates rows still running=1, and never re-enables a job
// that was disabled mid-flight (CASE keeps enabled=0).
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
	wantEnabled := 1
	nextStr := formatCronTime(now)
	expr := job.Expr
	if again {
		nextStr = formatCronTime(next)
		if newExpr != "" {
			expr = newExpr
		}
	} else {
		wantEnabled = 0
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE cron_job SET
			running = 0,
			enabled = CASE WHEN enabled = 0 THEN 0 ELSE ? END,
			expr = ?,
			next_run_at = ?,
			last_run_at = ?,
			last_error = ?,
			updated_at = ?
		WHERE id = ? AND running = 1`,
		wantEnabled, expr, nextStr, formatCronTime(now), errText, formatCronTime(now), job.ID)
	return err
}

// Defer clears running and moves next_run_at forward without finishing the job.
// Only applies while the job is still claimed (running=1), so Cancel wins races.
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
		WHERE id = ? AND running = 1`,
		formatCronTime(until.UTC()), reason, formatCronTime(now), id)
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
		formatCronTime(time.Now().UTC()), KindSparkPing, sessionID)
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

// EnsureSpark creates/refreshes the daily planner and seeds today's ping jobs
// at most once per local day. Reboots do not compound: once the planner already
// points at tomorrow (today was seeded), we only prune stale leftovers from
// prior days. The daily planner also CancelSparkPings before each new seed.
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
	tomorrowStart := addOneCalendarDay(startToday).UTC()
	// Seed remaining day now; planner wakes tomorrow (not today's start).
	template.Kind = KindSpark
	template.Expr = FormatSparkExpr(spec)
	template.NextRun = tomorrowStart
	template.Timezone = loc.String()

	existing, ok, err := s.FindSpark(ctx, delivery.SessionID)
	if err != nil {
		return Job{}, false, err
	}
	if ok && (existing.Prompt != prompt || existing.Expr != template.Expr) {
		if err := s.Cancel(ctx, existing.ID); err != nil {
			return Job{}, false, err
		}
		_, _ = s.CancelSparkPings(ctx, delivery.SessionID)
		ok = false
	}

	if !ok {
		job, err := s.Schedule(ctx, prompt, template, delivery)
		if err != nil {
			return Job{}, false, err
		}
		_ = s.disableExtraSparkPlanners(ctx, delivery.SessionID, job.ID)
		if err := s.seedSparkDay(ctx, prompt, spec, delivery, loc); err != nil {
			return job, true, err
		}
		return job, true, nil
	}

	job := existing
	_ = s.disableExtraSparkPlanners(ctx, delivery.SessionID, job.ID)

	// Drop leftovers from previous days so they cannot fire alongside today's plan.
	if _, err := s.CancelStaleSparkPings(ctx, delivery.SessionID, startToday); err != nil {
		return Job{}, false, err
	}

	// Already planned today (next wake is tomorrow or later): do not reseed.
	// This is what prevented "pending==0 on restart → another full roll".
	if !job.NextRunAt.Before(tomorrowStart) {
		return job, false, nil
	}

	// Planner still due for today (e.g. process was down at window start):
	// seed remaining day once and advance planner to tomorrow.
	if err := s.setNextRun(ctx, job.ID, template.NextRun); err != nil {
		return Job{}, false, err
	}
	job.NextRunAt = template.NextRun
	if err := s.seedSparkDay(ctx, prompt, spec, delivery, loc); err != nil {
		return job, false, err
	}
	return job, false, nil
}

func (s *Store) seedSparkDay(ctx context.Context, prompt string, spec SparkSpec, delivery Delivery, loc *time.Location) error {
	_, _ = s.CancelSparkPings(ctx, delivery.SessionID)
	_, times, err := PlanSparkDayTimes(spec, loc, time.Now())
	if err != nil {
		return err
	}
	_, err = s.ScheduleSparkPings(ctx, prompt, delivery, loc.String(), times)
	return err
}

// CancelStaleSparkPings disables pending spark_ping jobs scheduled before `before`
// (typically today's local window start), so prior-day leftovers cannot compound.
func (s *Store) CancelStaleSparkPings(ctx context.Context, sessionID string, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind = ? AND session_id = ? AND next_run_at < ?`,
		formatCronTime(time.Now().UTC()), KindSparkPing, sessionID, formatCronTime(before.UTC()))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// disableExtraSparkPlanners keeps a single enabled spark planner per session.
func (s *Store) disableExtraSparkPlanners(ctx context.Context, sessionID string, keepID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET enabled = 0, running = 0, updated_at = ?
		WHERE enabled = 1 AND kind = ? AND session_id = ? AND id != ?`,
		formatCronTime(time.Now().UTC()), KindSpark, sessionID, keepID)
	return err
}

func (s *Store) setNextRun(ctx context.Context, id int64, next time.Time) error {
	now := formatCronTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE cron_job SET next_run_at = ?, updated_at = ? WHERE id = ?`,
		formatCronTime(next.UTC()), now, id)
	return err
}

// formatCronTime uses fixed 9-digit nanos so lexicographic <= matches instant order.
func formatCronTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func parseCronTime(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02T15:04:05.000000000Z", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// addOneCalendarDay advances by one civil day in t's location (DST-safe vs Add(24h)).
func addOneCalendarDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day()+1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
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
	j.NextRunAt, _ = parseCronTime(next)
	j.CreatedAt, _ = parseCronTime(created)
	j.UpdatedAt, _ = parseCronTime(updated)
	if last.Valid {
		t, err := parseCronTime(last.String)
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
