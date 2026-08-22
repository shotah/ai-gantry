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
Voice: <8–12 short lines>

Facts: durable facts, preferences, open tasks, and names. Drop other chitchat
that is not a nickname, game, or running joke — those belong on Voice.
North-star aims (how you show up for months) belong in SELF.md (self_note), not Facts.
Progress, dates, and open loops belong in Facts / memory — not SELF.md.
Use absolute dates (2006-01-02) for anything time-bound — never yesterday, tomorrow,
this morning, or a weekday alone (those go stale when the week rolls).

Voice: today's register, nicknames in play, the current game, and running jokes.
Quote a joke's exact wording — a paraphrased joke is a dead joke. Keep up to 8
short verbatim quotes (each under ~100 characters).
Copy the prior Voice block forward UNCHANGED unless a new joke, nickname, or
game appeared in the dropped turns. When something new lands, add or replace
one line and keep the exact wording. Do not paraphrase existing quotes.
If the prior summary has no Voice: line, start one from the dropped turns
(or leave Voice empty if nothing tonal happened).`

const (
	maxFoldMsgChars   = 1200
	maxFoldTotalChars = 12000
	maxSummaryChars   = 4000
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

// VoiceDelta returns new Voice: bits that appeared in next vs prior.
// Empty when Voice was copied forward unchanged, or the only addition is
// mood weather ("dry today"). One line, suitable for a self_note append.
func VoiceDelta(prior, next string) string {
	_, priorVoice := LedgerParts(prior)
	_, nextVoice := LedgerParts(next)
	if strings.TrimSpace(nextVoice) == "" {
		return ""
	}
	if normalizeVoice(nextVoice) == normalizeVoice(priorVoice) {
		return ""
	}
	have := map[string]bool{}
	for _, b := range VoiceBits(priorVoice) {
		have[normalizeVoice(b)] = true
	}
	var added []string
	for _, b := range VoiceBits(nextVoice) {
		if b == "" || have[normalizeVoice(b)] || isMoodWeather(b) {
			continue
		}
		added = append(added, b)
	}
	return strings.Join(added, "; ")
}

// VoiceBits splits a Voice: body on newlines and semicolons, keeping
// quoted jokes intact.
func VoiceBits(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var bits []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		bits = append(bits, splitVoiceSemi(line)...)
	}
	return bits
}

func splitVoiceSemi(s string) []string {
	var out []string
	var b strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case r == ';' && !inQuote:
			if t := strings.TrimSpace(b.String()); t != "" {
				out = append(out, t)
			}
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if t := strings.TrimSpace(b.String()); t != "" {
		out = append(out, t)
	}
	if len(out) == 0 {
		return []string{strings.TrimSpace(s)}
	}
	return out
}

func normalizeVoice(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// isMoodWeather skips one-off register ("dry today") that the distill prompt
// already tells the model not to park in SELF.md.
func isMoodWeather(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if strings.Contains(s, `"`) {
		return false
	}
	lower := strings.ToLower(s)
	for _, keep := range []string{"gag", "joke", "nickname", "game", "ritual", "calls ", " is \""} {
		if strings.Contains(lower, keep) {
			return false
		}
	}
	return len(strings.Fields(s)) <= 3
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
