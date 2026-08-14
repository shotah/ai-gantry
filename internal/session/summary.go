package session

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/shotah/ai-gantry/internal/provider"
)

// Summarizer folds dropped history turns into a durable session summary.
type Summarizer interface {
	Fold(ctx context.Context, prior string, dropped []Message) (string, error)
}

// LLMSummarizer uses the chat completer for a cheap compression pass.
type LLMSummarizer struct {
	Completer provider.Completer
}

const summarizeSystem = `You maintain a rolling conversation summary for a personal agent.
Merge the dropped turns into the prior summary. Keep it short (one tight paragraph).
Preserve durable facts, preferences, open tasks, and names. Drop chitchat.
Use absolute dates (2006-01-02) for anything time-bound — never yesterday, tomorrow,
this morning, or a weekday alone (those go stale when the week rolls).
Reply with ONLY the updated summary text — no markdown fences, no preamble.`

const (
	maxFoldMsgChars   = 1200
	maxFoldTotalChars = 12000
	maxSummaryChars   = 2000
)

// Fold returns an updated summary paragraph.
func (s *LLMSummarizer) Fold(ctx context.Context, prior string, dropped []Message) (string, error) {
	if s == nil || s.Completer == nil {
		return prior, fmt.Errorf("session: summarizer not configured")
	}
	if len(dropped) == 0 {
		return prior, nil
	}
	var b strings.Builder
	if strings.TrimSpace(prior) != "" {
		b.WriteString("Prior summary:\n")
		b.WriteString(clipRunes(strings.TrimSpace(prior), maxSummaryChars))
		b.WriteString("\n\n")
	}
	b.WriteString("Dropped turns:\n")
	for _, m := range dropped {
		if b.Len() >= maxFoldTotalChars {
			b.WriteString("…[additional dropped turns omitted]\n")
			break
		}
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(clipRunes(m.Content, maxFoldMsgChars))
		b.WriteByte('\n')
	}
	res, err := s.Completer.Complete(ctx, provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: summarizeSystem},
			{Role: provider.RoleUser, Content: b.String()},
		},
	})
	if err != nil {
		return prior, err
	}
	out := strings.TrimSpace(res.Content)
	if out == "" {
		return prior, fmt.Errorf("session: empty summary from model")
	}
	return clipRunes(out, maxSummaryChars), nil
}

func clipRunes(s string, limit int) string {
	if limit < 1 || len(s) <= limit {
		return s
	}
	// Prefer rune-safe trim when possible; byte trim is fine for fold budgets.
	if limit > 1 {
		return s[:limit-1] + "…"
	}
	return s
}

// Summary returns the rolling summary for sessionID (empty if none).
func (s *Store) Summary(ctx context.Context, sessionID string) (string, error) {
	var summary string
	err := s.db.QueryRowContext(ctx, `
		SELECT summary FROM session WHERE id = ?`, sessionID).Scan(&summary)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("session: summary: %w", err)
	}
	return summary, nil
}
