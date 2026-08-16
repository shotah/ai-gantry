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
Merge the dropped turns into the prior summary. Reply with ONLY this shape
(no markdown fences, no preamble):

Facts: <one tight paragraph>
Voice: <2–4 short lines>

Facts: durable facts, preferences, open tasks, and names. Drop other chitchat.
Use absolute dates (2006-01-02) for anything time-bound — never yesterday, tomorrow,
this morning, or a weekday alone (those go stale when the week rolls).

Voice: today's register, nicknames in play, the current game, and running jokes.
Quote a joke's exact wording — a paraphrased joke is a dead joke. Keep up to 3
short verbatim quotes (each under ~100 characters).
Copy the prior Voice block forward UNCHANGED unless a new joke, nickname, or
game appeared in the dropped turns. When something new lands, add or replace
one line and keep the exact wording. Do not paraphrase existing quotes.
If the prior summary has no Voice: line, start one from the dropped turns
(or leave Voice empty if nothing tonal happened).`

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
	return clipRunes(ensureVoiceLedger(prior, out), maxSummaryChars), nil
}

const (
	factsLabel = "Facts:"
	voiceLabel = "Voice:"
)

// ensureVoiceLedger keeps a Facts:/Voice: shape even when a small model
// forgets the labels. Prior Voice is copied forward if the new text omits it.
func ensureVoiceLedger(prior, out string) string {
	facts, voice, hasVoice := splitLedger(out)
	if !hasVoice || strings.TrimSpace(voice) == "" {
		if _, priorVoice, priorHas := splitLedger(prior); priorHas && strings.TrimSpace(priorVoice) != "" {
			voice = strings.TrimSpace(priorVoice)
		}
	}
	if strings.TrimSpace(facts) == "" && strings.TrimSpace(voice) == "" {
		return strings.TrimSpace(out)
	}
	if strings.TrimSpace(voice) == "" {
		return factsLabel + " " + strings.TrimSpace(facts)
	}
	if strings.TrimSpace(facts) == "" {
		return voiceLabel + " " + strings.TrimSpace(voice)
	}
	return factsLabel + " " + strings.TrimSpace(facts) + "\n" + voiceLabel + " " + strings.TrimSpace(voice)
}

// LedgerParts splits a session summary into Facts and Voice.
// An unlabeled legacy paragraph is returned as facts with empty voice.
func LedgerParts(s string) (facts, voice string) {
	facts, voice, _ = splitLedger(s)
	return strings.TrimSpace(facts), strings.TrimSpace(voice)
}

func splitLedger(s string) (facts, voice string, hasVoice bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	voiceAt := indexLineLabel(s, voiceLabel)
	if voiceAt < 0 {
		if factsAt := indexLineLabel(s, factsLabel); factsAt == 0 {
			return strings.TrimSpace(s[len(factsLabel):]), "", false
		}
		return s, "", false
	}
	head := strings.TrimSpace(s[:voiceAt])
	voice = strings.TrimSpace(s[voiceAt+len(voiceLabel):])
	if factsAt := indexLineLabel(head, factsLabel); factsAt == 0 {
		facts = strings.TrimSpace(head[len(factsLabel):])
	} else {
		facts = head
	}
	return facts, voice, true
}

func indexLineLabel(s, label string) int {
	lower := strings.ToLower(s)
	want := strings.ToLower(label)
	if strings.HasPrefix(lower, want) {
		return 0
	}
	i := strings.Index(lower, "\n"+want)
	if i < 0 {
		return -1
	}
	return i + 1
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
