package mcp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// BudgetDB is the SQLite BudgetStore: one row per server per period on the
// shared gantry.db handle, so a monthly cap survives restarts and redeploys.
type BudgetDB struct {
	db *sql.DB
}

// OpenBudgetDB migrates the counter table on an already-open handle.
func OpenBudgetDB(db *sql.DB) (*BudgetDB, error) {
	if db == nil {
		return nil, errors.New("mcp: budget: nil db")
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS mcp_budget (
			server TEXT NOT NULL,
			period TEXT NOT NULL,
			calls  INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (server, period)
		)`); err != nil {
		return nil, fmt.Errorf("mcp: budget: migrate: %w", err)
	}
	return &BudgetDB{db: db}, nil
}

// Take is one conditional upsert — SQLite runs a statement atomically, so
// two tool calls in the same parallel batch cannot both squeeze through the
// last slot, and there is no read-then-write lock upgrade to go BUSY on.
// The WHERE only guards the update path; the insert path is the first call
// of the period and limit is at least 1 by construction.
func (s *BudgetDB) Take(ctx context.Context, server, period string, limit int) (int, bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO mcp_budget (server, period, calls) VALUES (?, ?, 1)
		ON CONFLICT(server, period) DO UPDATE SET calls = calls + 1
		WHERE mcp_budget.calls < ?`, server, period, limit)
	if err != nil {
		return 0, false, fmt.Errorf("mcp: budget: take: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("mcp: budget: take: %w", err)
	}
	used, err := s.Used(ctx, server, period)
	if err != nil {
		return 0, false, fmt.Errorf("mcp: budget: read: %w", err)
	}
	return used, n > 0, nil
}

// Used reports the count for a server's current bucket (0 when none).
func (s *BudgetDB) Used(ctx context.Context, server, period string) (int, error) {
	var used int
	err := s.db.QueryRowContext(ctx, `SELECT calls FROM mcp_budget WHERE server = ? AND period = ?`, server, period).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return used, err
}
