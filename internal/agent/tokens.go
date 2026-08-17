package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/session"
)

// formatTokens prints a chars/4 estimate breakdown of the standing prompt
// (persona + summary + history + hydration + schemas). Volatile this-turn
// user/clock text is excluded — that is what /perf volatile measures.
func (a *Agent) formatTokens(ctx context.Context, sessionID string) (string, error) {
	personaEst := estChars(a.personaText())

	summary, err := a.sessions.Summary(ctx, sessionID)
	if err != nil {
		return "", err
	}
	summaryEst := 0
	if s := strings.TrimSpace(summary); s != "" {
		summaryEst = estChars("[session summary]\n" + s)
	}

	n, histEst, err := a.sessions.Stats(ctx, sessionID)
	if err != nil {
		return "", err
	}

	schemaEst := 0
	if a.tools != nil {
		schemaEst = mcp.EstimateSchemaBudget(a.publishedTools(ctx, sessionID)).EstTokens
	}

	hydrateEst := 0
	hydrateNote := "off"
	if a.memory != nil {
		query := lastSessionUserText(ctx, a.sessions, sessionID)
		loc, _ := a.clockZone()
		entries, err := a.memory.Hydrate(ctx, query, 30)
		if err != nil {
			a.log.Warn("tokens hydrate failed", "err", err)
			hydrateNote = "error"
		} else {
			hydrateNote = fmt.Sprintf("%d rows", len(entries))
			if block := memory.FormatHydration(entries, loc); block != "" {
				hydrateEst = estChars(block)
			}
		}
	}

	standing := personaEst + summaryEst + histEst + hydrateEst + schemaEst
	var b strings.Builder
	b.WriteString("tokens (chars/4 estimates)\n")
	fmt.Fprintf(&b, "  persona     %d\n", personaEst)
	fmt.Fprintf(&b, "  summary     %d\n", summaryEst)
	fmt.Fprintf(&b, "  history     %d  (%d msgs)\n", histEst, n)
	fmt.Fprintf(&b, "  hydration   %d  (%s)\n", hydrateEst, hydrateNote)
	fmt.Fprintf(&b, "  schemas     %d\n", schemaEst)
	fmt.Fprintf(&b, "  standing    %d", standing)
	return b.String(), nil
}

func lastSessionUserText(ctx context.Context, hist History, sessionID string) string {
	if hist == nil {
		return ""
	}
	msgs, err := hist.Messages(ctx, sessionID)
	if err != nil {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == session.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

func estChars(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}
