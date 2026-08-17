package mcpenable

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Row is one enabled prefix for a session.
type Row struct {
	SessionID string
	Prefix    string
	Hold      string
	Source    string
	LastUsed  time.Time
}

// Store persists prefix enable rows in gantry.db.
type Store struct {
	db *sql.DB
}

// OpenDB attaches the mcp_enable table to an existing handle.
func OpenDB(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("mcpenable: nil db")
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS mcp_enable (
			session_id TEXT NOT NULL,
			prefix     TEXT NOT NULL,
			hold       TEXT NOT NULL,
			source     TEXT NOT NULL DEFAULT 'agent',
			last_used  TEXT NOT NULL,
			PRIMARY KEY (session_id, prefix)
		)`)
	if err != nil {
		return fmt.Errorf("mcpenable: migrate: %w", err)
	}
	return nil
}

func stamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseStamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// List returns non-expired rows for sessionID.
func (s *Store) List(ctx context.Context, sessionID string, now time.Time) ([]Row, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if err := s.Expire(ctx, now); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, prefix, hold, source, last_used
		FROM mcp_enable WHERE session_id = ?`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("mcpenable: list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Row
	for rows.Next() {
		var r Row
		var used string
		if err := rows.Scan(&r.SessionID, &r.Prefix, &r.Hold, &r.Source, &used); err != nil {
			return nil, fmt.Errorf("mcpenable: scan: %w", err)
		}
		t, err := parseStamp(used)
		if err != nil {
			continue
		}
		r.LastUsed = t
		out = append(out, r)
	}
	return out, rows.Err()
}

// Expire deletes idle rows (short 27h / long 76h).
func (s *Store) Expire(ctx context.Context, now time.Time) error {
	now = now.UTC()
	shortCut := stamp(now.Add(-ShortIdle))
	longCut := stamp(now.Add(-LongIdle))
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM mcp_enable WHERE
			(hold = ? AND last_used < ?) OR
			(hold = ? AND last_used < ?)`,
		HoldShort, shortCut, HoldLong, longCut)
	if err != nil {
		return fmt.Errorf("mcpenable: expire: %w", err)
	}
	return nil
}

// Enable upserts prefixes. hold is short|long. source is agent|human.
func (s *Store) Enable(ctx context.Context, sessionID string, prefixes []string, hold, source string, now time.Time, index []string) (landed, failed []string, err error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil, fmt.Errorf("mcpenable: empty session_id")
	}
	hold = normalizeHold(hold)
	if source != SourceHuman {
		source = SourceAgent
	}
	now = now.UTC()
	if len(prefixes) > MaxEnableList {
		return nil, prefixes, fmt.Errorf("mcpenable: at most %d prefixes per call", MaxEnableList)
	}

	existing, err := s.List(ctx, sessionID, now)
	if err != nil {
		return nil, nil, err
	}
	longN := 0
	for _, r := range existing {
		if r.Hold == HoldLong {
			longN++
		}
	}

	for _, raw := range prefixes {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if !indexHas(index, key) {
			failed = append(failed, key+" (unknown prefix)")
			continue
		}
		cur, ok := findRow(existing, key)
		nextSource := source
		nextHold := hold
		if ok && cur.Source == SourceHuman && source == SourceAgent && hold == HoldLong && cur.Hold == HoldShort {
			failed = append(failed, key+" (operator set short)")
			continue
		}
		if ok && cur.Source == SourceHuman && source == SourceAgent {
			nextSource = SourceHuman
			if hold == HoldLong && cur.Hold == HoldShort {
				failed = append(failed, key+" (operator set short)")
				continue
			}
			if hold == HoldShort {
				nextHold = HoldShort
			} else {
				nextHold = cur.Hold
			}
		}
		if nextHold == HoldLong && HasSubprefixes(key, index) {
			failed = append(failed, key+" (long refuses a fat server; use a family prefix)")
			continue
		}
		if nextHold == HoldLong && (!ok || cur.Hold != HoldLong) {
			if longN >= MaxLong {
				failed = append(failed, key+" (long cap reached)")
				continue
			}
			longN++
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO mcp_enable (session_id, prefix, hold, source, last_used)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(session_id, prefix) DO UPDATE SET
				hold = excluded.hold,
				source = excluded.source,
				last_used = excluded.last_used`,
			sessionID, key, nextHold, nextSource, stamp(now)); err != nil {
			return landed, failed, fmt.Errorf("mcpenable: enable %s: %w", key, err)
		}
		landed = append(landed, key)
		if !ok {
			existing = append(existing, Row{Prefix: key, Hold: nextHold, Source: nextSource})
		} else {
			for i := range existing {
				if existing[i].Prefix == key {
					existing[i].Hold = nextHold
					existing[i].Source = nextSource
				}
			}
		}
	}
	return landed, failed, nil
}

// Off deletes a prefix now.
func (s *Store) Off(ctx context.Context, sessionID, prefix string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM mcp_enable WHERE session_id = ? AND prefix = ?`,
		strings.TrimSpace(sessionID), strings.TrimSpace(prefix))
	if err != nil {
		return fmt.Errorf("mcpenable: off: %w", err)
	}
	return nil
}

// Touch refreshes last_used on the longest matching row.
func (s *Store) Touch(ctx context.Context, sessionID, toolName string, now time.Time) error {
	rows, err := s.List(ctx, sessionID, now)
	if err != nil {
		return err
	}
	var keys []string
	for _, r := range rows {
		keys = append(keys, r.Prefix)
	}
	key, ok := LongestKey(toolName, keys)
	if !ok {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE mcp_enable SET last_used = ? WHERE session_id = ? AND prefix = ?`,
		stamp(now.UTC()), sessionID, key)
	return err
}

func findRow(rows []Row, prefix string) (Row, bool) {
	for _, r := range rows {
		if r.Prefix == prefix {
			return r, true
		}
	}
	return Row{}, false
}

func indexHas(index []string, key string) bool {
	for _, k := range index {
		if k == key {
			return true
		}
	}
	return false
}
