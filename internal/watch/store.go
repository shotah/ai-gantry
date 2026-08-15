package watch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

	"github.com/shotah/ai-gantry/internal/cron"
)

// Watch is one polled subscription.
type Watch struct {
	ID              int64
	Tool            string
	Args            json.RawMessage
	Label           string
	IntervalSeconds int
	NextRunAt       time.Time
	SessionID       string
	UserID          string
	ChatID          string
	ThreadID        int
	SeenIDs         []string
	Enabled         bool
	Running         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastRunAt       *time.Time
	LastError       string
}

// Store persists watches in gantry.db.
type Store struct {
	db         *sql.DB
	maxWatches int
}

// OpenDB attaches the watch schema to an existing DB handle.
func OpenDB(db *sql.DB, maxWatches int) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("watch: nil db")
	}
	if maxWatches < 1 {
		maxWatches = 50
	}
	s := &Store{db: db, maxWatches: maxWatches}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS watch (
			id               INTEGER PRIMARY KEY,
			tool             TEXT NOT NULL,
			args             TEXT NOT NULL DEFAULT '{}',
			label            TEXT NOT NULL DEFAULT '',
			interval_seconds INTEGER NOT NULL,
			next_run_at      TEXT NOT NULL,
			session_id       TEXT NOT NULL,
			user_id          TEXT NOT NULL,
			chat_id          TEXT NOT NULL DEFAULT '',
			thread_id        INTEGER NOT NULL DEFAULT 0,
			seen_ids         TEXT NOT NULL DEFAULT '[]',
			enabled          INTEGER NOT NULL DEFAULT 1,
			running          INTEGER NOT NULL DEFAULT 0,
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL,
			last_run_at      TEXT,
			last_error       TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_watch_due
			ON watch(enabled, running, next_run_at)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("watch: migrate: %w", err)
		}
	}
	return nil
}

// MaxWatches returns the configured cap.
func (s *Store) MaxWatches() int { return s.maxWatches }

// ActiveCount returns enabled watches.
func (s *Store) ActiveCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch WHERE enabled = 1`).Scan(&n)
	return n, err
}

// Add inserts a watch. Next run is now so the first tick can seed the cursor.
func (s *Store) Add(ctx context.Context, tool string, args json.RawMessage, label string, interval time.Duration, delivery cron.Delivery) (Watch, error) {
	tool = strings.TrimSpace(tool)
	if err := validateTool(tool); err != nil {
		return Watch{}, err
	}
	if delivery.SessionID == "" {
		return Watch{}, fmt.Errorf("watch: delivery session_id is required")
	}
	if interval < MinInterval {
		return Watch{}, fmt.Errorf("watch: interval must be >= %s", MinInterval)
	}
	args = normalizeArgs(args)
	n, err := s.ActiveCount(ctx)
	if err != nil {
		return Watch{}, err
	}
	if n >= s.maxWatches {
		return Watch{}, fmt.Errorf("watch: max active watches (%d) reached", s.maxWatches)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO watch (
			tool, args, label, interval_seconds, next_run_at,
			session_id, user_id, chat_id, thread_id,
			seen_ids, enabled, running, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', 1, 0, ?, ?)`,
		tool, string(args), strings.TrimSpace(label), int(interval.Seconds()),
		formatTime(now),
		delivery.SessionID, delivery.UserID, delivery.ChatID, delivery.ThreadID,
		formatTime(now), formatTime(now),
	)
	if err != nil {
		return Watch{}, fmt.Errorf("watch: insert: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.Get(ctx, id)
}

func validateTool(tool string) error {
	if tool == "" {
		return fmt.Errorf("watch: tool is required")
	}
	if !strings.Contains(tool, "__") {
		return fmt.Errorf("watch: tool %q must be a prefixed MCP name (server__tool)", tool)
	}
	return nil
}

func normalizeArgs(args json.RawMessage) json.RawMessage {
	if len(args) == 0 || string(args) == "null" {
		return json.RawMessage(`{}`)
	}
	var obj map[string]any
	if err := json.Unmarshal(args, &obj); err != nil {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// Get loads one watch by id.
func (s *Store) Get(ctx context.Context, id int64) (Watch, error) {
	row := s.db.QueryRowContext(ctx, watchSelect+` WHERE id = ?`, id)
	w, err := scanWatch(row)
	if err == sql.ErrNoRows {
		return Watch{}, fmt.Errorf("watch: %d not found", id)
	}
	return w, err
}

const watchSelect = `
		SELECT id, tool, args, label, interval_seconds, next_run_at,
		       session_id, user_id, chat_id, thread_id, seen_ids,
		       enabled, running, created_at, updated_at, last_run_at, last_error
		FROM watch`

// ListSession returns watches for sessionID (empty = all).
func (s *Store) ListSession(ctx context.Context, sessionID string, includeDisabled bool) ([]Watch, error) {
	q := watchSelect + ` WHERE 1=1`
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
	return scanWatches(rows)
}

// ClearStaleRunning resets running=1 left by a crash.
func (s *Store) ClearStaleRunning(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE watch SET running = 0, updated_at = ? WHERE running = 1`, formatTime(time.Now().UTC()))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Cancel disables a watch by id.
func (s *Store) Cancel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE watch SET enabled = 0, running = 0, updated_at = ? WHERE id = ?`,
		formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("watch: %d not found", id)
	}
	return nil
}

// Due returns enabled, idle watches with next_run_at <= now.
func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]Watch, error) {
	if limit < 1 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, watchSelect+`
		WHERE enabled = 1 AND running = 0 AND next_run_at <= ?
		ORDER BY next_run_at ASC LIMIT ?`, formatTime(now.UTC()), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanWatches(rows)
}

// Claim marks a watch running if it is still due and idle.
func (s *Store) Claim(ctx context.Context, id int64, now time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE watch SET running = 1, updated_at = ?
		WHERE id = ? AND enabled = 1 AND running = 0 AND next_run_at <= ?`,
		formatTime(now.UTC()), id, formatTime(now.UTC()))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Finish clears running, stores the cursor, and schedules the next tick.
// Cancel-safe: never re-enables a watch disabled mid-flight.
func (s *Store) Finish(ctx context.Context, w Watch, seen []string, runErr error) error {
	now := time.Now().UTC()
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
		if len(errText) > 500 {
			errText = errText[:500]
		}
	}
	if seen == nil {
		seen = w.SeenIDs
	}
	seenJSON, err := json.Marshal(seen)
	if err != nil {
		return err
	}
	interval := time.Duration(w.IntervalSeconds) * time.Second
	if interval < MinInterval {
		interval = DefaultInterval
	}
	next := now.Add(interval)
	_, err = s.db.ExecContext(ctx, `
		UPDATE watch SET
			running = 0,
			enabled = CASE WHEN enabled = 0 THEN 0 ELSE 1 END,
			seen_ids = ?,
			next_run_at = ?,
			last_run_at = ?,
			last_error = ?,
			updated_at = ?
		WHERE id = ? AND running = 1`,
		string(seenJSON), formatTime(next), formatTime(now), errText, formatTime(now), w.ID)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWatch(row rowScanner) (Watch, error) {
	var (
		w                          Watch
		args, seenRaw              string
		nextRun, created, updated  string
		lastRun                    sql.NullString
		enabled, running, threadID int
	)
	err := row.Scan(
		&w.ID, &w.Tool, &args, &w.Label, &w.IntervalSeconds, &nextRun,
		&w.SessionID, &w.UserID, &w.ChatID, &threadID, &seenRaw,
		&enabled, &running, &created, &updated, &lastRun, &w.LastError,
	)
	if err != nil {
		return Watch{}, err
	}
	w.Args = json.RawMessage(args)
	w.ThreadID = threadID
	w.Enabled = enabled == 1
	w.Running = running == 1
	w.SeenIDs = parseSeen(seenRaw)
	if w.NextRunAt, err = parseTime(nextRun); err != nil {
		return Watch{}, err
	}
	if w.CreatedAt, err = parseTime(created); err != nil {
		return Watch{}, err
	}
	if w.UpdatedAt, err = parseTime(updated); err != nil {
		return Watch{}, err
	}
	if lastRun.Valid && lastRun.String != "" {
		t, err := parseTime(lastRun.String)
		if err == nil {
			w.LastRunAt = &t
		}
	}
	return w, nil
}

func scanWatches(rows *sql.Rows) ([]Watch, error) {
	var out []Watch
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func parseSeen(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02T15:04:05.000000000Z", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// ForceDueForTest sets next_run_at to the past so the next poll claims the watch.
func (s *Store) ForceDueForTest(ctx context.Context, id int64) error {
	past := formatTime(time.Now().UTC().Add(-time.Minute))
	res, err := s.db.ExecContext(ctx, `
		UPDATE watch SET next_run_at = ?, running = 0, updated_at = ? WHERE id = ?`,
		past, formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("watch: %d not found", id)
	}
	return nil
}
