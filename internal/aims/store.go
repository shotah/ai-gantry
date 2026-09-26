// Package aims is the ledger of events scored against live aim/<area> rows.
package aims

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

	"github.com/shotah/ai-gantry/internal/memory"
)

const (
	// ScoreMin is the lowest agent opinion of an event.
	ScoreMin = -3
	// ScoreMax is the highest agent opinion of an event.
	ScoreMax = 3

	dayLayout     = "2006-01-02"
	defaultHist   = 50
	sourceDefault = "model"
)

// Event is one thing that happened, scored against every aim it touches.
type Event struct {
	ID           int64
	Day          string // local YYYY-MM-DD
	What         string
	Ref          string
	Metric       string
	Value        *float64
	Unit         string
	Source       string
	Note         string // applied to every score written this Log
	CreatedAt    time.Time
	SupersededBy *int64
	Scores       []Score
}

// Score is the agent's opinion of an event toward one aim.
type Score struct {
	Area  string
	Value int
	Note  string
}

// Block is a dated span for later adherence math.
type Block struct {
	Area    string
	FromDay string
	ToDay   string
}

// Store persists the ledger on the shared gantry.db handle.
type Store struct {
	db  *sql.DB
	loc *time.Location
	mem memory.Memory
}

// OpenDB attaches the aims schema to an existing DB handle.
// loc is CRON_TZ (nil = UTC). mem supplies live aim/<area> rows; nil
// treats the board as empty and Log rejects every area.
func OpenDB(db *sql.DB, loc *time.Location, mem memory.Memory) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("aims: nil db")
	}
	if loc == nil {
		loc = time.UTC
	}
	s := &Store{db: db, loc: loc, mem: mem}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS aim_event (
			id            INTEGER PRIMARY KEY,
			day           TEXT NOT NULL,
			what          TEXT NOT NULL,
			ref           TEXT NOT NULL DEFAULT '',
			metric        TEXT NOT NULL DEFAULT '',
			value         REAL,
			unit          TEXT NOT NULL DEFAULT '',
			source        TEXT NOT NULL,
			created_at    TEXT NOT NULL,
			superseded_by INTEGER
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS aim_event_ref
			ON aim_event(ref) WHERE ref != '' AND superseded_by IS NULL`,
		`CREATE INDEX IF NOT EXISTS aim_event_day ON aim_event(day)`,
		`CREATE TABLE IF NOT EXISTS aim_score (
			event_id INTEGER NOT NULL REFERENCES aim_event(id),
			area     TEXT NOT NULL,
			score    INTEGER NOT NULL,
			note     TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (event_id, area)
		)`,
		`CREATE TABLE IF NOT EXISTS aim_block (
			area     TEXT PRIMARY KEY,
			from_day TEXT NOT NULL,
			to_day   TEXT NOT NULL
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("aims: migrate: %w", err)
		}
	}
	return nil
}

// Areas returns live aim/<area> slugs (aim/bootstrap omitted).
func (s *Store) Areas(ctx context.Context) ([]string, error) {
	if s == nil || s.mem == nil {
		return nil, nil
	}
	rows, err := s.mem.ListBySubjectPrefix(ctx, memory.KindInsight, memory.SubjectAimPrefix, 0)
	if err != nil {
		return nil, fmt.Errorf("aims: areas: %w", err)
	}
	out := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, e := range rows {
		area := strings.TrimPrefix(strings.TrimSpace(e.Subject), memory.SubjectAimPrefix)
		area = strings.TrimSpace(area)
		if area == "" || area == "bootstrap" {
			continue
		}
		if _, ok := seen[area]; ok {
			continue
		}
		seen[area] = struct{}{}
		out = append(out, area)
	}
	sort.Strings(out)
	return out, nil
}

// Log inserts an event scored against each area in scores.
// A set Ref supersedes the live row with that ref; a set ID supersedes
// that live event. Blank fields on a rewrite copy from the old row.
// New scores overlay the old map; other areas keep their scores.
func (s *Store) Log(ctx context.Context, ev Event, scores map[string]int) (Event, error) {
	if s == nil || s.db == nil {
		return Event{}, fmt.Errorf("aims: nil store")
	}
	if len(scores) == 0 {
		return Event{}, fmt.Errorf("aims: scores required")
	}
	live, err := s.Areas(ctx)
	if err != nil {
		return Event{}, err
	}
	liveSet := map[string]struct{}{}
	for _, a := range live {
		liveSet[a] = struct{}{}
	}
	norm := make(map[string]int, len(scores))
	for area, n := range scores {
		area = strings.TrimSpace(area)
		if area == "" {
			return Event{}, fmt.Errorf("aims: empty area")
		}
		if n < ScoreMin || n > ScoreMax {
			return Event{}, fmt.Errorf("aims: score for %s must be %d..%d, got %d", area, ScoreMin, ScoreMax, n)
		}
		if _, ok := liveSet[area]; !ok {
			return Event{}, fmt.Errorf("aims: unknown area %q (live: %s)", area, joinAreas(live))
		}
		norm[area] = n
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, fmt.Errorf("aims: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var old Event
	switch {
	case ev.ID > 0:
		old, err = getEventTx(ctx, tx, ev.ID)
		if err != nil {
			return Event{}, err
		}
		if old.SupersededBy != nil {
			return Event{}, fmt.Errorf("aims: event %d is superseded", ev.ID)
		}
	case strings.TrimSpace(ev.Ref) != "":
		old, err = getLiveByRefTx(ctx, tx, strings.TrimSpace(ev.Ref))
		if err != nil && err != sql.ErrNoRows {
			return Event{}, err
		}
		if err == sql.ErrNoRows {
			old = Event{}
		}
	}

	merged := overlayEvent(old, ev, s.loc)
	if merged.What == "" {
		return Event{}, fmt.Errorf("aims: what is required")
	}
	if err := parseDay(merged.Day); err != nil {
		return Event{}, err
	}

	scoreMap := map[string]Score{}
	for _, sc := range old.Scores {
		scoreMap[sc.Area] = sc
	}
	note := strings.TrimSpace(ev.Note)
	for area, n := range norm {
		sc := Score{Area: area, Value: n, Note: note}
		if note == "" {
			if prev, ok := scoreMap[area]; ok {
				sc.Note = prev.Note
			}
		}
		scoreMap[area] = sc
	}

	if old.ID > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE aim_event SET superseded_by = id WHERE id = ? AND superseded_by IS NULL`, old.ID); err != nil {
			return Event{}, fmt.Errorf("aims: retire: %w", err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, `
		INSERT INTO aim_event (day, what, ref, metric, value, unit, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		merged.Day, merged.What, merged.Ref, merged.Metric, nullFloat(merged.Value),
		merged.Unit, merged.Source, now)
	if err != nil {
		return Event{}, fmt.Errorf("aims: insert: %w", err)
	}
	id, _ := res.LastInsertId()
	if old.ID > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE aim_event SET superseded_by = ? WHERE id = ?`, id, old.ID); err != nil {
			return Event{}, fmt.Errorf("aims: supersede: %w", err)
		}
	}

	areas := make([]string, 0, len(scoreMap))
	for area := range scoreMap {
		areas = append(areas, area)
	}
	sort.Strings(areas)
	for _, area := range areas {
		sc := scoreMap[area]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO aim_score (event_id, area, score, note) VALUES (?, ?, ?, ?)`,
			id, area, sc.Value, sc.Note); err != nil {
			return Event{}, fmt.Errorf("aims: score: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Event{}, fmt.Errorf("aims: commit: %w", err)
	}
	return s.Get(ctx, id)
}

// Get loads one event (live or superseded) with its scores.
func (s *Store) Get(ctx context.Context, id int64) (Event, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, day, what, ref, metric, value, unit, source, created_at, superseded_by
		FROM aim_event WHERE id = ?`, id)
	ev, err := scanEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Event{}, fmt.Errorf("aims: event %d not found", id)
		}
		return Event{}, err
	}
	scores, err := s.scoresFor(ctx, []int64{id})
	if err != nil {
		return Event{}, err
	}
	ev.Scores = scores[id]
	return ev, nil
}

// History returns live events newest first. Empty area lists every aim.
// from and to are inclusive YYYY-MM-DD; empty means unbounded. limit < 1
// is 50. Matching events include every area's scores, not only the filter.
func (s *Store) History(ctx context.Context, area, from, to string, limit int) ([]Event, error) {
	area = strings.TrimSpace(area)
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from != "" {
		if err := parseDay(from); err != nil {
			return nil, err
		}
	}
	if to != "" {
		if err := parseDay(to); err != nil {
			return nil, err
		}
	}
	if limit < 1 {
		limit = defaultHist
	}

	q := `
		SELECT DISTINCT e.id, e.day, e.what, e.ref, e.metric, e.value, e.unit, e.source, e.created_at, e.superseded_by
		FROM aim_event e
		JOIN aim_score s ON s.event_id = e.id
		WHERE e.superseded_by IS NULL`
	args := []any{}
	if area != "" {
		q += ` AND s.area = ?`
		args = append(args, area)
	}
	if from != "" {
		q += ` AND e.day >= ?`
		args = append(args, from)
	}
	if to != "" {
		q += ` AND e.day <= ?`
		args = append(args, to)
	}
	q += ` ORDER BY e.day DESC, e.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("aims: history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []Event
	var ids []int64
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
		ids = append(ids, ev.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	byID, err := s.scoresFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range events {
		events[i].Scores = byID[events[i].ID]
	}
	return events, nil
}

// Forget drops scores for area. An event with no scores left is superseded.
func (s *Store) Forget(ctx context.Context, area string) error {
	area = strings.TrimSpace(area)
	if area == "" {
		return fmt.Errorf("aims: empty area")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("aims: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM aim_score WHERE area = ?`, area); err != nil {
		return fmt.Errorf("aims: forget scores: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE aim_event SET superseded_by = id
		WHERE superseded_by IS NULL
		  AND id NOT IN (SELECT event_id FROM aim_score)`); err != nil {
		return fmt.Errorf("aims: forget orphans: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("aims: commit: %w", err)
	}
	return nil
}

// Block returns the dated span for area, if any.
func (s *Store) Block(ctx context.Context, area string) (Block, bool, error) {
	area = strings.TrimSpace(area)
	if area == "" {
		return Block{}, false, fmt.Errorf("aims: empty area")
	}
	var b Block
	err := s.db.QueryRowContext(ctx, `
		SELECT area, from_day, to_day FROM aim_block WHERE area = ?`, area).
		Scan(&b.Area, &b.FromDay, &b.ToDay)
	if err == sql.ErrNoRows {
		return Block{}, false, nil
	}
	if err != nil {
		return Block{}, false, err
	}
	return b, true, nil
}

// SetBlock writes the dated span for area.
func (s *Store) SetBlock(ctx context.Context, area, from, to string) error {
	area = strings.TrimSpace(area)
	if area == "" {
		return fmt.Errorf("aims: empty area")
	}
	if err := parseDay(from); err != nil {
		return err
	}
	if err := parseDay(to); err != nil {
		return err
	}
	if from > to {
		return fmt.Errorf("aims: block from %s is after to %s", from, to)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO aim_block (area, from_day, to_day) VALUES (?, ?, ?)
		ON CONFLICT(area) DO UPDATE SET from_day = excluded.from_day, to_day = excluded.to_day`,
		area, from, to)
	if err != nil {
		return fmt.Errorf("aims: set block: %w", err)
	}
	return nil
}

func overlayEvent(old, in Event, loc *time.Location) Event {
	out := old
	out.ID = 0
	out.SupersededBy = nil
	out.Scores = nil
	if w := strings.TrimSpace(in.What); w != "" {
		out.What = w
	}
	if d := strings.TrimSpace(in.Day); d != "" {
		out.Day = d
	}
	if out.Day == "" {
		out.Day = time.Now().In(loc).Format(dayLayout)
	}
	if r := strings.TrimSpace(in.Ref); r != "" {
		out.Ref = r
	}
	if m := strings.TrimSpace(in.Metric); m != "" {
		out.Metric = m
		out.Value = in.Value
		out.Unit = strings.TrimSpace(in.Unit)
	} else if in.Value != nil {
		out.Value = in.Value
		if u := strings.TrimSpace(in.Unit); u != "" {
			out.Unit = u
		}
	}
	if src := strings.TrimSpace(in.Source); src != "" {
		out.Source = src
	}
	if out.Source == "" {
		out.Source = sourceDefault
	}
	out.What = strings.TrimSpace(out.What)
	out.Ref = strings.TrimSpace(out.Ref)
	out.Metric = strings.TrimSpace(out.Metric)
	out.Unit = strings.TrimSpace(out.Unit)
	return out
}

func (s *Store) scoresFor(ctx context.Context, ids []int64) (map[int64][]Score, error) {
	out := map[int64][]Score{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	ph := make([]string, len(ids))
	for i, id := range ids {
		args[i] = id
		ph[i] = "?"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, area, score, note FROM aim_score
		WHERE event_id IN (`+strings.Join(ph, ",")+`)
		ORDER BY area`, args...)
	if err != nil {
		return nil, fmt.Errorf("aims: scores: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var sc Score
		if err := rows.Scan(&id, &sc.Area, &sc.Value, &sc.Note); err != nil {
			return nil, err
		}
		out[id] = append(out[id], sc)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(row scanner) (Event, error) {
	var e Event
	var val sql.NullFloat64
	var created string
	var sup sql.NullInt64
	err := row.Scan(&e.ID, &e.Day, &e.What, &e.Ref, &e.Metric, &val, &e.Unit, &e.Source, &created, &sup)
	if err != nil {
		return Event{}, err
	}
	if val.Valid {
		v := val.Float64
		e.Value = &v
	}
	if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
		e.CreatedAt = t
	}
	if sup.Valid {
		id := sup.Int64
		e.SupersededBy = &id
	}
	return e, nil
}

func getEventTx(ctx context.Context, tx *sql.Tx, id int64) (Event, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, day, what, ref, metric, value, unit, source, created_at, superseded_by
		FROM aim_event WHERE id = ?`, id)
	ev, err := scanEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Event{}, fmt.Errorf("aims: event %d not found", id)
		}
		return Event{}, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT area, score, note FROM aim_score WHERE event_id = ? ORDER BY area`, id)
	if err != nil {
		return Event{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sc Score
		if err := rows.Scan(&sc.Area, &sc.Value, &sc.Note); err != nil {
			return Event{}, err
		}
		ev.Scores = append(ev.Scores, sc)
	}
	return ev, rows.Err()
}

func getLiveByRefTx(ctx context.Context, tx *sql.Tx, ref string) (Event, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, day, what, ref, metric, value, unit, source, created_at, superseded_by
		FROM aim_event WHERE ref = ? AND superseded_by IS NULL`, ref)
	ev, err := scanEvent(row)
	if err != nil {
		return Event{}, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT area, score, note FROM aim_score WHERE event_id = ? ORDER BY area`, ev.ID)
	if err != nil {
		return Event{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sc Score
		if err := rows.Scan(&sc.Area, &sc.Value, &sc.Note); err != nil {
			return Event{}, err
		}
		ev.Scores = append(ev.Scores, sc)
	}
	return ev, rows.Err()
}

func parseDay(day string) error {
	day = strings.TrimSpace(day)
	if day == "" {
		return fmt.Errorf("aims: empty day")
	}
	if _, err := time.Parse(dayLayout, day); err != nil {
		return fmt.Errorf("aims: bad day %q", day)
	}
	return nil
}

func nullFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func joinAreas(areas []string) string {
	if len(areas) == 0 {
		return "none"
	}
	return strings.Join(areas, ", ")
}
