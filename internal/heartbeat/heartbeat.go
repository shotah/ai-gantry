// Package heartbeat writes a singleton SQLite row for Docker healthchecks.
package heartbeat

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const singletonID = 1

// DefaultInterval is how often the daemon refreshes the heartbeat row.
const DefaultInterval = 15 * time.Second

// DefaultMaxAge is how stale the row may be before `gantry status` fails.
// Compose healthcheck interval is 30s with 3 retries; 60s is a comfortable bound.
const DefaultMaxAge = 60 * time.Second

// Store persists the heartbeat singleton in gantry.db.
type Store struct {
	db *sql.DB
}

// OpenDB attaches the heartbeat table to an existing DB handle.
func OpenDB(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("heartbeat: nil db")
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS heartbeat (
			id         INTEGER PRIMARY KEY CHECK (id = 1),
			updated_at TEXT NOT NULL,
			version    TEXT NOT NULL DEFAULT ''
		)`)
	if err != nil {
		return fmt.Errorf("heartbeat: migrate: %w", err)
	}
	return nil
}

// Touch upserts the singleton row with the current time.
func (s *Store) Touch(ctx context.Context, version string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO heartbeat (id, updated_at, version) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			updated_at = excluded.updated_at,
			version = excluded.version`,
		singletonID, now, version)
	if err != nil {
		return fmt.Errorf("heartbeat: touch: %w", err)
	}
	return nil
}

// Check reports whether the heartbeat is fresh (within maxAge).
func (s *Store) Check(ctx context.Context, maxAge time.Duration) error {
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	var updated string
	err := s.db.QueryRowContext(ctx, `
		SELECT updated_at FROM heartbeat WHERE id = ?`, singletonID).Scan(&updated)
	if err == sql.ErrNoRows {
		return fmt.Errorf("heartbeat: no row (daemon not running?)")
	}
	if err != nil {
		return fmt.Errorf("heartbeat: read: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		t, err = time.Parse(time.RFC3339, updated)
		if err != nil {
			return fmt.Errorf("heartbeat: bad updated_at %q", updated)
		}
	}
	age := time.Since(t)
	if age > maxAge {
		return fmt.Errorf("heartbeat: stale (age=%s max=%s)", age.Truncate(time.Second), maxAge)
	}
	return nil
}

// Start pulses Touch on interval until ctx is cancelled.
func (s *Store) Start(ctx context.Context, interval time.Duration, version string, log *slog.Logger) {
	if s == nil {
		return
	}
	if interval <= 0 {
		interval = DefaultInterval
	}
	if log == nil {
		log = slog.Default()
	}
	if err := s.Touch(ctx, version); err != nil {
		log.Warn("heartbeat initial touch failed", "err", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Touch(ctx, version); err != nil {
				log.Warn("heartbeat touch failed", "err", err)
			}
		}
	}
}
