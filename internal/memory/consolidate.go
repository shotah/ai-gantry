package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/shotah/ai-gantry/internal/provider"
)

const (
	defaultConsolidateBatch = 20
	maxConsolidateAttempts  = 3
	consolidatedDone        = 1
	consolidatedQuarantine  = 2
)

// Consolidator runs the cheap "sleep cycle" over unconsolidated episodes.
// Builtin backend only.
type Consolidator struct {
	Store     *Builtin
	Completer provider.Completer
	Interval  time.Duration
	BatchSize int
	Logger    *slog.Logger

	mu      sync.Mutex
	lastRun time.Time
	lastErr error
}

// Start runs consolidation passes on Interval until ctx is cancelled.
// Interval <= 0 means disabled (returns immediately).
func (c *Consolidator) Start(ctx context.Context) {
	if c == nil || c.Store == nil || c.Completer == nil || c.Interval <= 0 {
		return
	}
	log := c.Logger
	if log == nil {
		log = slog.Default()
	}
	batch := c.BatchSize
	if batch < 1 {
		batch = defaultConsolidateBatch
	}

	log.Info("memory consolidator started", "interval", c.Interval.String(), "batch", batch)
	// One pass on boot so restarts do not wait a full interval.
	c.Pass(ctx)
	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("memory consolidator stopped")
			return
		case <-ticker.C:
			c.Pass(ctx)
		}
	}
}

// Pass runs one consolidation cycle (bounded batch).
func (c *Consolidator) Pass(ctx context.Context) {
	if c == nil || c.Store == nil || c.Completer == nil {
		return
	}
	log := c.Logger
	if log == nil {
		log = slog.Default()
	}
	batch := c.BatchSize
	if batch < 1 {
		batch = defaultConsolidateBatch
	}
	c.runPass(ctx, log, batch)
}

func (c *Consolidator) runPass(ctx context.Context, log *slog.Logger, batch int) {
	var passErr error
	defer func() {
		c.mu.Lock()
		c.lastRun = time.Now()
		c.lastErr = passErr
		c.mu.Unlock()
	}()

	episodes, err := c.Store.ListUnconsolidatedEpisodes(ctx, batch)
	if err != nil {
		passErr = err
		log.Warn("memory consolidate list failed", "err", err)
		return
	}
	if len(episodes) == 0 {
		log.Debug("memory consolidate: nothing to do")
		return
	}

	ids := episodeIDs(episodes)
	allowed := make(map[int64]bool, len(ids))
	for _, id := range ids {
		allowed[id] = true
	}

	prompt := buildConsolidatePrompt(episodes)
	res, err := c.Completer.Complete(ctx, provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: consolidateSystem},
			{Role: provider.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		passErr = err
		log.Warn("memory consolidate llm failed", "err", err)
		return
	}

	items, err := parseConsolidateJSON(res.Content)
	if err != nil {
		passErr = err
		log.Warn("memory consolidate parse failed", "err", err, "raw", truncate(res.Content, 200))
		if qerr := c.Store.RecordConsolidateFailure(ctx, ids, maxConsolidateAttempts); qerr != nil {
			log.Warn("memory consolidate failure record failed", "err", qerr)
		}
		return
	}

	stored, err := c.Store.ApplyConsolidation(ctx, ids, items, allowed)
	if err != nil {
		passErr = err
		log.Warn("memory consolidate apply failed", "err", err)
		return
	}
	log.Info("memory consolidate pass",
		"episodes", len(episodes),
		"extracted", len(items),
		"stored", stored,
	)
}

// LastStatus returns the time and error from the most recent consolidation pass.
// at.IsZero means no pass has completed yet.
func (c *Consolidator) LastStatus() (at time.Time, err error) {
	if c == nil {
		return time.Time{}, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastRun, c.lastErr
}

const consolidateSystem = `You consolidate episodic memories into durable structured rows.
Reply with ONLY a JSON array (no markdown). Each element:
{"kind":"fact|preference|person|insight","subject":"...","content":"...","supersedes":[id,...]}
Deduplicate: if a new row replaces an older memory id from this batch, list that id in supersedes.
If nothing durable, return [].
Keep content atomic (one statement). Do not invent facts not implied by episodes.`

type consolidateItem struct {
	Kind       string  `json:"kind"`
	Subject    string  `json:"subject"`
	Content    string  `json:"content"`
	Supersedes []int64 `json:"supersedes"`
}

func buildConsolidatePrompt(episodes []Entry) string {
	var b strings.Builder
	b.WriteString("Unconsolidated episodes:\n")
	for _, e := range episodes {
		_, _ = fmt.Fprintf(&b, "- id=%d subject=%q: %s\n", e.ID, e.Subject, e.Content)
	}
	return b.String()
}

func parseConsolidateJSON(raw string) ([]consolidateItem, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty consolidator reply")
	}
	// Strip optional ```json fences
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```JSON")
		raw = strings.TrimPrefix(raw, "```")
		if i := strings.LastIndex(raw, "```"); i >= 0 {
			raw = raw[:i]
		}
		raw = strings.TrimSpace(raw)
	}
	if raw == "" {
		return nil, fmt.Errorf("empty consolidator reply after fence strip")
	}
	var items []consolidateItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	return items, nil
}

func episodeIDs(episodes []Entry) []int64 {
	ids := make([]int64, len(episodes))
	for i, e := range episodes {
		ids[i] = e.ID
	}
	return ids
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ApplyConsolidation stores durable rows and marks episodes in one transaction.
// allowedSupersede restricts supersede targets to the prompted episode batch.
func (b *Builtin) ApplyConsolidation(ctx context.Context, episodeIDs []int64, items []consolidateItem, allowedSupersede map[int64]bool) (int, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	stored := 0
	for _, item := range items {
		kind := strings.ToLower(strings.TrimSpace(item.Kind))
		if kind == "" {
			kind = KindFact
		}
		if err := ValidateKind(kind); err != nil {
			continue
		}
		if kind == KindEpisode {
			continue
		}
		subject := strings.TrimSpace(item.Subject)
		content := strings.TrimSpace(item.Content)
		if subject == "" || content == "" {
			continue
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO memory (kind, subject, content, source, confidence, created_at, updated_at, consolidated)
			VALUES (?, ?, ?, ?, 1.0, ?, ?, 1)`,
			kind, subject, content, SourceConsolidation, now, now,
		)
		if err != nil {
			return 0, err
		}
		newID, _ := res.LastInsertId()
		stored++
		for _, oldID := range item.Supersedes {
			if oldID <= 0 || !allowedSupersede[oldID] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE memory SET superseded_by = ?, updated_at = ? WHERE id = ?`,
				newID, now, oldID); err != nil {
				return 0, err
			}
		}
	}

	for _, id := range episodeIDs {
		if _, err := tx.ExecContext(ctx, `
			UPDATE memory SET consolidated = ?, updated_at = ? WHERE id = ?`,
			consolidatedDone, now, id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return stored, nil
}

// RecordConsolidateFailure bumps attempts; quarantines after maxAttempts.
func (b *Builtin) RecordConsolidateFailure(ctx context.Context, ids []int64, maxAttempts int) error {
	if maxAttempts < 1 {
		maxAttempts = maxConsolidateAttempts
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, id := range ids {
		if _, err := b.db.ExecContext(ctx, `
			UPDATE memory SET
				consolidate_attempts = consolidate_attempts + 1,
				consolidated = CASE
					WHEN consolidate_attempts + 1 >= ? THEN ?
					ELSE consolidated
				END,
				updated_at = ?
			WHERE id = ? AND consolidated = 0`,
			maxAttempts, consolidatedQuarantine, now, id); err != nil {
			return err
		}
	}
	return nil
}
