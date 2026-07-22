package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const (
	defaultHydrateLimit = 30
	defaultRecallLimit  = 20
	episodeTTL          = 30 * 24 * time.Hour
)

// Builtin is the SQLite + FTS5 memory backend in gantry.db.
type Builtin struct {
	db    *sql.DB
	owned bool // close db on Close when we opened it
}

// Open opens (or creates) memory tables in dataDir/gantry.db.
func Open(dataDir string) (*Builtin, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("memory: mkdir: %w", err)
	}
	dbPath := filepath.Join(dataDir, "gantry.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("memory: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	b := &Builtin{db: db, owned: true}
	if err := b.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return b, nil
}

// OpenDB attaches memory schema to an existing DB handle (shared with session).
func OpenDB(db *sql.DB) (*Builtin, error) {
	if db == nil {
		return nil, fmt.Errorf("memory: nil db")
	}
	b := &Builtin{db: db, owned: false}
	if err := b.migrate(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Builtin) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA foreign_keys=ON;`,
		`CREATE TABLE IF NOT EXISTS memory (
			id            INTEGER PRIMARY KEY,
			kind          TEXT NOT NULL,
			subject       TEXT NOT NULL,
			content       TEXT NOT NULL,
			source        TEXT NOT NULL,
			confidence    REAL NOT NULL DEFAULT 1.0,
			created_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL,
			expires_at    TEXT,
			superseded_by INTEGER,
			consolidated  INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_active
			ON memory(kind, updated_at DESC);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
			subject, content,
			content='memory',
			content_rowid='id'
		);`,
		`CREATE TRIGGER IF NOT EXISTS memory_ai AFTER INSERT ON memory BEGIN
			INSERT INTO memory_fts(rowid, subject, content)
			VALUES (new.id, new.subject, new.content);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_ad AFTER DELETE ON memory BEGIN
			INSERT INTO memory_fts(memory_fts, rowid, subject, content)
			VALUES('delete', old.id, old.subject, old.content);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_au AFTER UPDATE ON memory BEGIN
			INSERT INTO memory_fts(memory_fts, rowid, subject, content)
			VALUES('delete', old.id, old.subject, old.content);
			INSERT INTO memory_fts(rowid, subject, content)
			VALUES (new.id, new.subject, new.content);
		END;`,
	}
	for _, q := range stmts {
		if _, err := b.db.Exec(q); err != nil {
			return fmt.Errorf("memory: migrate: %w", err)
		}
	}
	return nil
}

// Close closes the DB when Open owned it.
func (b *Builtin) Close() error {
	if b == nil || b.db == nil || !b.owned {
		return nil
	}
	return b.db.Close()
}

// Store inserts one atomic memory row.
func (b *Builtin) Store(ctx context.Context, kind, subject, content string) (Entry, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if err := ValidateKind(kind); err != nil {
		return Entry{}, err
	}
	subject = strings.TrimSpace(subject)
	content = strings.TrimSpace(content)
	if subject == "" || content == "" {
		return Entry{}, fmt.Errorf("memory: subject and content are required")
	}

	now := time.Now().UTC()
	var expires any
	if kind == KindEpisode {
		exp := now.Add(episodeTTL)
		expires = exp.Format(time.RFC3339Nano)
	}

	res, err := b.db.ExecContext(ctx, `
		INSERT INTO memory (kind, subject, content, source, confidence, created_at, updated_at, expires_at, consolidated)
		VALUES (?, ?, ?, ?, 1.0, ?, ?, ?, 0)`,
		kind, subject, content, SourceChat,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), expires,
	)
	if err != nil {
		return Entry{}, fmt.Errorf("memory: store: %w", err)
	}
	id, _ := res.LastInsertId()
	return b.get(ctx, id)
}

// Recall runs FTS5 + recency ranking.
func (b *Builtin) Recall(ctx context.Context, query string, limit int) ([]Entry, error) {
	if limit < 1 {
		limit = defaultRecallLimit
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return b.listActive(ctx, limit)
	}
	return b.search(ctx, query, limit)
}

// Forget hard-deletes a row by id.
func (b *Builtin) Forget(ctx context.Context, id int64) error {
	res, err := b.db.ExecContext(ctx, `DELETE FROM memory WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("memory: forget: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: id %d not found", id)
	}
	return nil
}

// ForgetQuery deletes FTS matches for query (correctability).
func (b *Builtin) ForgetQuery(ctx context.Context, query string) (int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, fmt.Errorf("memory: forget query is required")
	}
	entries, err := b.search(ctx, query, 100)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if err := b.Forget(ctx, e.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Hydrate returns active durable rows plus FTS hits for query (cap ~30).
func (b *Builtin) Hydrate(ctx context.Context, query string, limit int) ([]Entry, error) {
	if limit < 1 {
		limit = defaultHydrateLimit
	}
	seen := map[int64]struct{}{}
	var out []Entry

	add := func(list []Entry) {
		for _, e := range list {
			if _, ok := seen[e.ID]; ok {
				continue
			}
			seen[e.ID] = struct{}{}
			out = append(out, e)
			if len(out) >= limit {
				return
			}
		}
	}

	durable, err := b.listDurable(ctx, limit)
	if err != nil {
		return nil, err
	}
	add(durable)
	if len(out) >= limit {
		return out[:limit], nil
	}
	if q := strings.TrimSpace(query); q != "" {
		hits, err := b.search(ctx, q, limit)
		if err != nil {
			return nil, err
		}
		add(hits)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListUnconsolidatedEpisodes returns episode rows awaiting consolidation.
func (b *Builtin) ListUnconsolidatedEpisodes(ctx context.Context, limit int) ([]Entry, error) {
	if limit < 1 {
		limit = 20
	}
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory
		WHERE kind = ? AND consolidated = 0 AND superseded_by IS NULL
		  AND (expires_at IS NULL OR expires_at > ?)
		ORDER BY created_at ASC
		LIMIT ?`, KindEpisode, time.Now().UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

// MarkConsolidated marks episode ids as processed by the consolidator.
func (b *Builtin) MarkConsolidated(ctx context.Context, ids []int64) error {
	for _, id := range ids {
		if _, err := b.db.ExecContext(ctx, `
			UPDATE memory SET consolidated = 1, updated_at = ? WHERE id = ?`,
			time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
			return err
		}
	}
	return nil
}

// StoreConsolidated inserts a consolidation-sourced row.
func (b *Builtin) StoreConsolidated(ctx context.Context, kind, subject, content string) (Entry, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if err := ValidateKind(kind); err != nil {
		return Entry{}, err
	}
	now := time.Now().UTC()
	res, err := b.db.ExecContext(ctx, `
		INSERT INTO memory (kind, subject, content, source, confidence, created_at, updated_at, consolidated)
		VALUES (?, ?, ?, ?, 1.0, ?, ?, 1)`,
		kind, strings.TrimSpace(subject), strings.TrimSpace(content), SourceConsolidation,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Entry{}, err
	}
	id, _ := res.LastInsertId()
	return b.get(ctx, id)
}

// Supersede links oldID → newID without deleting.
func (b *Builtin) Supersede(ctx context.Context, oldID, newID int64) error {
	_, err := b.db.ExecContext(ctx, `
		UPDATE memory SET superseded_by = ?, updated_at = ? WHERE id = ?`,
		newID, time.Now().UTC().Format(time.RFC3339Nano), oldID)
	return err
}

func (b *Builtin) listActive(ctx context.Context, limit int) ([]Entry, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory
		WHERE superseded_by IS NULL
		  AND (expires_at IS NULL OR expires_at > ?)
		ORDER BY updated_at DESC
		LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (b *Builtin) listDurable(ctx context.Context, limit int) ([]Entry, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory
		WHERE superseded_by IS NULL
		  AND kind IN (?, ?, ?)
		  AND (expires_at IS NULL OR expires_at > ?)
		ORDER BY
			CASE kind WHEN 'preference' THEN 0 WHEN 'person' THEN 1 ELSE 2 END,
			updated_at DESC
		LIMIT ?`, KindPreference, KindPerson, KindFact, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (b *Builtin) search(ctx context.Context, query string, limit int) ([]Entry, error) {
	fts := ftsQuery(query)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := b.db.QueryContext(ctx, `
		SELECT m.id, m.kind, m.subject, m.content, m.source, m.confidence,
		       m.created_at, m.updated_at, m.expires_at, m.superseded_by
		FROM memory_fts
		JOIN memory m ON m.id = memory_fts.rowid
		WHERE memory_fts MATCH ?
		  AND m.superseded_by IS NULL
		  AND (m.expires_at IS NULL OR m.expires_at > ?)
		ORDER BY rank, m.updated_at DESC
		LIMIT ?`, fts, now, limit)
	if err != nil {
		return b.searchLike(ctx, query, now, limit)
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (b *Builtin) searchLike(ctx context.Context, query, now string, limit int) ([]Entry, error) {
	like := "%" + query + "%"
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory
		WHERE superseded_by IS NULL
		  AND (expires_at IS NULL OR expires_at > ?)
		  AND (subject LIKE ? OR content LIKE ?)
		ORDER BY updated_at DESC
		LIMIT ?`, now, like, like, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: search: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (b *Builtin) get(ctx context.Context, id int64) (Entry, error) {
	row := b.db.QueryRowContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory WHERE id = ?`, id)
	return scanEntry(row)
}

func ftsQuery(q string) string {
	parts := strings.Fields(q)
	if len(parts) == 0 {
		return `""`
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ReplaceAll(p, `"`, "")
		if p == "" {
			continue
		}
		out = append(out, `"`+p+`"`)
	}
	return strings.Join(out, " ")
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(row rowScanner) (Entry, error) {
	var e Entry
	var created, updated string
	var expires sql.NullString
	var superseded sql.NullInt64
	if err := row.Scan(&e.ID, &e.Kind, &e.Subject, &e.Content, &e.Source, &e.Confidence,
		&created, &updated, &expires, &superseded); err != nil {
		return Entry{}, err
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if expires.Valid {
		t, err := time.Parse(time.RFC3339Nano, expires.String)
		if err == nil {
			e.ExpiresAt = &t
		}
	}
	if superseded.Valid {
		v := superseded.Int64
		e.SupersededBy = &v
	}
	return e, nil
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
