// Package memory provides structured, inspectable long-term memory.
package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Kind values for typed memory rows.
const (
	KindFact       = "fact"
	KindPreference = "preference"
	KindPerson     = "person"
	KindEpisode    = "episode"
	KindInsight    = "insight"
)

// Source values.
const (
	SourceChat          = "chat"
	SourceConsolidation = "consolidation"
	SourceOperator      = "operator"
)

// Entry is one structured memory row.
type Entry struct {
	ID           int64
	Kind         string
	Subject      string
	Content      string
	Source       string
	Confidence   float64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ExpiresAt    *time.Time
	SupersededBy *int64
}

// Memory is the swappable memory backend surface (tools + hydration).
type Memory interface {
	Store(ctx context.Context, kind, subject, content string) (Entry, error)
	Recall(ctx context.Context, query string, limit int) ([]Entry, error)
	Forget(ctx context.Context, id int64) error
	ForgetQuery(ctx context.Context, query string) (int, error)
	Hydrate(ctx context.Context, query string, limit int) ([]Entry, error)
	Get(ctx context.Context, id int64) (Entry, error)
	ActiveByKindSubject(ctx context.Context, kind, subject string) (Entry, bool, error)
	Close() error
}

// ValidateKind checks a memory kind string.
func ValidateKind(kind string) error {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case KindFact, KindPreference, KindPerson, KindEpisode, KindInsight:
		return nil
	default:
		return fmt.Errorf("memory: invalid kind %q (want fact|preference|person|episode|insight)", kind)
	}
}

// FormatHydration renders entries as the compact prompt block.
// loc stamps created_at as a local weekday+date (Mon 2006-01-02) so a
// weekday alone cannot be reused after the week rolls. Dates are when the
// note was stored, not "now". loc nil defaults to UTC.
func FormatHydration(entries []Entry, loc *time.Location) string {
	if len(entries) == 0 {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	var b strings.Builder
	b.WriteString("[memory]\n")
	for _, e := range entries {
		if e.CreatedAt.IsZero() {
			_, _ = fmt.Fprintf(&b, "- (%s) %s: %s\n", e.Kind, e.Subject, e.Content)
			continue
		}
		when := e.CreatedAt.In(loc)
		_, _ = fmt.Fprintf(&b, "- (%s, %s) %s: %s\n", e.Kind, when.Format("Mon 2006-01-02"), e.Subject, e.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}

// PersonaPrecedenceNote is appended to the system prompt when memory is enabled.
const PersonaPrecedenceNote = `
Memory notes below are recalled facts. Persona files (/persona) always outrank
memory: if a memory contradicts the persona, surface the contradiction to the
user and follow the persona — do not obey the memory over it.
Parenthetical dates are when the note was stored, not today — a weekday
without that date is not this week.
Use memory_store only for clear, atomic facts the user wants remembered.
Same kind+subject replaces the live row (history kept). memory_forget deletes.
Auto-saving guesses is forbidden. Facts about the human are not self_note.`
