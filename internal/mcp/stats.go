package mcp

import (
	"sort"
	"sync"
	"time"
)

// toolStatsTopN caps /toolstats output (const, not an env knob).
const toolStatsListCap = 15

// CallStats is a snapshot of cumulative MCP tool accounting since boot.
type CallStats struct {
	TotalCalls       int
	PrefixAlias      int
	ConstrainedRetry int
	UnknownTool      int
	ByTool           []ToolCallStat // sorted by total duration desc
}

// ToolCallStat is one resolved tool's counters.
type ToolCallStat struct {
	Name       string
	Calls      int
	Errors     int
	TotalDurMS int64
	MaxDurMS   int64
}

type toolCounters struct {
	calls      int
	errors     int
	totalDurMS int64
	maxDurMS   int64
}

type callStatsState struct {
	mu               sync.Mutex
	byTool           map[string]*toolCounters
	totalCalls       int
	prefixAlias      int
	constrainedRetry int
	unknownTool      int
}

func (h *Host) initCallStats() {
	h.stats = callStatsState{byTool: make(map[string]*toolCounters)}
}

func (h *Host) recordPrefixAlias() {
	h.stats.mu.Lock()
	h.stats.prefixAlias++
	h.stats.mu.Unlock()
}

func (h *Host) recordUnknownTool(constrained bool) {
	h.stats.mu.Lock()
	h.stats.unknownTool++
	if constrained {
		h.stats.constrainedRetry++
	}
	h.stats.mu.Unlock()
}

func (h *Host) recordToolCall(name string, dur time.Duration, failed bool) {
	ms := dur.Milliseconds()
	h.stats.mu.Lock()
	defer h.stats.mu.Unlock()
	h.stats.totalCalls++
	c := h.stats.byTool[name]
	if c == nil {
		c = &toolCounters{}
		h.stats.byTool[name] = c
	}
	c.calls++
	c.totalDurMS += ms
	if ms > c.maxDurMS {
		c.maxDurMS = ms
	}
	if failed {
		c.errors++
	}
}

// CallStats returns a snapshot of tool call counters (newest accounting since boot).
func (h *Host) CallStats() CallStats {
	h.stats.mu.Lock()
	defer h.stats.mu.Unlock()
	out := CallStats{
		TotalCalls:       h.stats.totalCalls,
		PrefixAlias:      h.stats.prefixAlias,
		ConstrainedRetry: h.stats.constrainedRetry,
		UnknownTool:      h.stats.unknownTool,
		ByTool:           make([]ToolCallStat, 0, len(h.stats.byTool)),
	}
	for name, c := range h.stats.byTool {
		out.ByTool = append(out.ByTool, ToolCallStat{
			Name:       name,
			Calls:      c.calls,
			Errors:     c.errors,
			TotalDurMS: c.totalDurMS,
			MaxDurMS:   c.maxDurMS,
		})
	}
	sort.Slice(out.ByTool, func(i, j int) bool {
		if out.ByTool[i].TotalDurMS != out.ByTool[j].TotalDurMS {
			return out.ByTool[i].TotalDurMS > out.ByTool[j].TotalDurMS
		}
		return out.ByTool[i].Name < out.ByTool[j].Name
	})
	if len(out.ByTool) > toolStatsListCap {
		out.ByTool = out.ByTool[:toolStatsListCap]
	}
	return out
}
