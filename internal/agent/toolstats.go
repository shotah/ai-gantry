package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/mcp"
)

const toolStatsTopN = 15

const helpText = `/new — reset this session's history
/cancel — stop the in-flight turn
/status — uptime, model, history, tools, turns
/tools — prefixed tool catalog (published vs available)
/brief /short /off — prefix hold (6h / 27h / drop now)
/examples — one capability idea (on|off for proactive pings)
/engagement /spark — looking-after-you wakes (on|off|3-5)
/perf — last turns: invocations, tools, batch, recoveries
/memstats — memory row counts and consolidation
/toolstats — per-tool call ledger since boot
/tokens — prompt token breakdown (estimates)
/auth — remote OAuth (url / paste code; see guide)
/help — this list`

func (a *Agent) formatToolStats() string {
	src, ok := a.tools.(interface{ CallStats() mcp.CallStats })
	if !ok {
		return "no tool calls yet"
	}
	stats := src.CallStats()
	if stats.TotalCalls == 0 && stats.PrefixAlias == 0 && stats.ConstrainedRetry == 0 && stats.UnknownTool == 0 {
		return "no tool calls yet"
	}
	uptime := formatUptime(time.Since(a.startedAt))
	var b strings.Builder
	fmt.Fprintf(&b, "tool stats — %d calls since boot (%s)\n", stats.TotalCalls, uptime)
	for i, row := range stats.ByTool {
		if i >= toolStatsTopN {
			break
		}
		okCalls := row.Calls - row.Errors
		if okCalls < 0 {
			okCalls = 0
		}
		avg := int64(0)
		if row.Calls > 0 {
			avg = row.TotalDurMS / int64(row.Calls)
		}
		fmt.Fprintf(&b, "%-24s %d calls  ✓%d ✗%d  avg %s  max %s\n",
			row.Name, row.Calls, okCalls, row.Errors,
			formatSec(avg), formatSec(row.MaxDurMS),
		)
	}
	fmt.Fprintf(&b, "repairs: prefix_alias=%d  constrained_retry=%d  unknown_tool=%d",
		stats.PrefixAlias, stats.ConstrainedRetry, stats.UnknownTool)
	return b.String()
}
