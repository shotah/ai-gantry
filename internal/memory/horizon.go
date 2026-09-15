package memory

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// Subject prefixes stamped onto [harness] every turn (hydration is lossy).
const (
	SubjectAimPrefix     = "aim/"
	SubjectWaitingPrefix = "waiting/"
	SubjectFollowPrefix  = "follow/"
	harnessHorizonMax    = 5
	harnessHorizonClip   = 72
	// horizonFetch is the ListBySubjectPrefix default window: wide enough
	// that the "+N more" overflow count is honest for a real board.
	horizonFetch = 30
	// loopStaleAfter: an open loop untouched this long gets a resolve-or-forget
	// cue so the [loops] cap does not fill with dead loops.
	loopStaleAfter = 21 * 24 * time.Hour
)

// ErrNotSupported is returned by backends that cannot answer a lookup
// (MCP memory has no live-row-by-subject). Callers skip the stamp instead
// of treating "not found" as "ask the human again".
var ErrNotSupported = errors.New("memory: not supported on this backend")

// FormatAims is the per-turn [aims] line. Empty if there are no live aim/ rows.
// North-star sentences stay in SELF.md; this is the tracker (insight + aim/<area>).
// now stamps "(12d ago)" from updated_at; zero now omits ages.
func FormatAims(entries []Entry, now time.Time) string {
	parts, more := horizonParts(entries, SubjectAimPrefix, now, 0)
	if len(parts) == 0 {
		return ""
	}
	return "[aims] " + strings.Join(parts, " · ") + horizonMore(more, "memory_recall aim/")
}

// FormatLoops is the per-turn [loops] line for open loops. waiting/ and
// follow/ are interleaved so five open waits cannot hide every follow.
// Loops untouched past three weeks carry a resolve-or-forget cue.
func FormatLoops(waiting, follow []Entry, now time.Time) string {
	parts, more := horizonParts(interleave(waiting, follow), "", now, loopStaleAfter)
	if len(parts) == 0 {
		return ""
	}
	return "[loops] " + strings.Join(parts, " · ") + horizonMore(more, "memory_recall waiting/ follow/")
}

func horizonMore(n int, hint string) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(" (+%d more — %s)", n, hint)
}

func interleave(a, b []Entry) []Entry {
	out := make([]Entry, 0, len(a)+len(b))
	for i := 0; i < len(a) || i < len(b); i++ {
		if i < len(a) {
			out = append(out, a[i])
		}
		if i < len(b) {
			out = append(out, b[i])
		}
	}
	return out
}

// horizonParts renders up to harnessHorizonMax entries and returns how many
// were cut so the line can say so instead of silently truncating.
func horizonParts(entries []Entry, stripPrefix string, now time.Time, staleAfter time.Duration) ([]string, int) {
	if len(entries) == 0 {
		return nil, 0
	}
	more := 0
	if len(entries) > harnessHorizonMax {
		more = len(entries) - harnessHorizonMax
		entries = entries[:harnessHorizonMax]
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		label := strings.TrimSpace(e.Subject)
		if stripPrefix != "" {
			label = strings.TrimPrefix(label, stripPrefix)
		}
		if label == "" {
			continue
		}
		part := label
		if body := clipHorizon(e.Content); body != "" {
			part += ": " + body
		}
		parts = append(parts, part+horizonAge(e, now, staleAfter))
	}
	return parts, more
}

// horizonAge is " (12d ago)" from updated_at (created_at fallback). Nothing
// inside the first day — a fresh row needs no age. Past staleAfter (>0) it
// adds the resolve-or-forget cue.
func horizonAge(e Entry, now time.Time, staleAfter time.Duration) string {
	at := e.UpdatedAt
	if at.IsZero() {
		at = e.CreatedAt
	}
	if at.IsZero() || now.IsZero() {
		return ""
	}
	d := now.Sub(at)
	if d < 24*time.Hour {
		return ""
	}
	s := " (" + channel.Age(d)
	if staleAfter > 0 && d >= staleAfter {
		s += " — resolve or memory_forget"
	}
	return s + ")"
}

func clipHorizon(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= harnessHorizonClip {
		return s
	}
	return string(r[:harnessHorizonClip-1]) + "…"
}
