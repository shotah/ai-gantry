package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/provider"
)

const defaultConsolidateBatch = 20

// Consolidator runs the cheap "sleep cycle" over unconsolidated episodes.
// Builtin backend only.
type Consolidator struct {
	Store     *Builtin
	Completer provider.Completer
	Interval  time.Duration
	BatchSize int
	Logger    *slog.Logger
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
	episodes, err := c.Store.ListUnconsolidatedEpisodes(ctx, batch)
	if err != nil {
		log.Warn("memory consolidate list failed", "err", err)
		return
	}
	if len(episodes) == 0 {
		log.Debug("memory consolidate: nothing to do")
		return
	}

	prompt := buildConsolidatePrompt(episodes)
	res, err := c.Completer.Complete(ctx, provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: consolidateSystem},
			{Role: provider.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		log.Warn("memory consolidate llm failed", "err", err)
		return
	}

	items, err := parseConsolidateJSON(res.Content)
	if err != nil {
		log.Warn("memory consolidate parse failed", "err", err, "raw", truncate(res.Content, 200))
		// Still mark episodes so a bad model reply doesn't spin forever.
		ids := episodeIDs(episodes)
		if markErr := c.Store.MarkConsolidated(ctx, ids); markErr != nil {
			log.Warn("memory consolidate mark failed", "err", markErr)
		}
		return
	}

	for _, item := range items {
		kind := strings.ToLower(strings.TrimSpace(item.Kind))
		if kind == "" {
			kind = KindFact
		}
		if err := ValidateKind(kind); err != nil {
			continue
		}
		if kind == KindEpisode {
			continue // consolidator promotes out of episodes
		}
		subject := strings.TrimSpace(item.Subject)
		content := strings.TrimSpace(item.Content)
		if subject == "" || content == "" {
			continue
		}
		neu, err := c.Store.StoreConsolidated(ctx, kind, subject, content)
		if err != nil {
			log.Warn("memory consolidate store failed", "err", err)
			continue
		}
		for _, oldID := range item.Supersedes {
			if oldID > 0 {
				_ = c.Store.Supersede(ctx, oldID, neu.ID)
			}
		}
	}

	if err := c.Store.MarkConsolidated(ctx, episodeIDs(episodes)); err != nil {
		log.Warn("memory consolidate mark failed", "err", err)
		return
	}
	log.Info("memory consolidate pass",
		"episodes", len(episodes),
		"extracted", len(items),
	)
}

const consolidateSystem = `You consolidate episodic memories into durable structured rows.
Reply with ONLY a JSON array (no markdown). Each element:
{"kind":"fact|preference|person|insight","subject":"...","content":"...","supersedes":[id,...]}
Deduplicate: if a new row replaces an older memory id, list that id in supersedes.
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
		return nil, nil
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
