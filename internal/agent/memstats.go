package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/shotah/ai-gantry/internal/memory"
)

func (a *Agent) formatMemStats(ctx context.Context) string {
	if a.memory == nil {
		return "memory: disabled"
	}
	builtin, ok := a.memory.(*memory.Builtin)
	if !ok {
		// MCP (or other) backend — row counts live elsewhere; consolidation is off.
		var b strings.Builder
		b.WriteString("memory: mcp backend (no local row counts)\n")
		b.WriteString("consolidation: off")
		return b.String()
	}
	snap, err := builtin.Stats(ctx)
	if err != nil {
		return fmt.Sprintf("memory: stats failed: %v", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "memory: %d rows  fact=%d preference=%d person=%d insight=%d episode=%d\n",
		snap.Total,
		snap.ByKind[memory.KindFact],
		snap.ByKind[memory.KindPreference],
		snap.ByKind[memory.KindPerson],
		snap.ByKind[memory.KindInsight],
		snap.ByKind[memory.KindEpisode],
	)
	fmt.Fprintf(&b, "state: active=%d expired=%d superseded=%d\n",
		snap.Active, snap.Expired, snap.Superseded)

	if a.consolidator == nil {
		b.WriteString("consolidation: off\n")
	} else {
		at, lastErr := a.consolidator.LastStatus()
		status := "ok"
		if lastErr != nil {
			status = lastErr.Error()
		}
		lastRun := "never"
		if !at.IsZero() {
			when := at
			if a.loc != nil {
				when = when.In(a.loc)
			}
			lastRun = when.Format("15:04:05")
		}
		fmt.Fprintf(&b, "consolidation: backlog=%d episodes  quarantined=%d  last_run=%s %s\n",
			snap.Backlog, snap.Quarantined, lastRun, status)
	}
	fmt.Fprintf(&b, "db: %s (WAL)", formatBytes(snap.DBBytes))
	return b.String()
}

func formatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const mb = 1024 * 1024
	if n >= mb {
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
	const kb = 1024
	if n >= kb {
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	}
	return fmt.Sprintf("%d B", n)
}
