package agent

import (
	"fmt"

	"github.com/shotah/ai-gantry/internal/provider"
)

// keepRecentToolResults is how many trailing tool payloads stay in full.
// Older tool results collapse to a one-line marker (readme §6 bounding rules).
// Search-heavy MCPs (flights/rentals/cars) re-send results each iteration, so
// keeping 2 (not 4) cuts in-turn prefill without losing the latest evidence.
const keepRecentToolResults = 2

// collapseOldToolResults shortens tool payloads older than the recent window.
// Within the window, older results that share a tool name with a newer one are
// also collapsed so repeated flights__offers_search calls do not stack.
func collapseOldToolResults(messages []provider.Message) []provider.Message {
	var toolIdx []int
	for i, m := range messages {
		if m.Role == provider.RoleTool {
			toolIdx = append(toolIdx, i)
		}
	}
	if len(toolIdx) == 0 {
		return messages
	}
	names := toolCallNames(messages)
	newestByName := make(map[string]int, len(toolIdx))
	for k := len(toolIdx) - 1; k >= 0; k-- {
		i := toolIdx[k]
		name := names[messages[i].ToolCallID]
		if name == "" {
			name = "result"
		}
		if _, ok := newestByName[name]; !ok {
			newestByName[name] = i
		}
	}
	cutoff := len(toolIdx) - keepRecentToolResults
	if cutoff < 0 {
		cutoff = 0
	}
	recent := make(map[int]bool, keepRecentToolResults)
	for _, i := range toolIdx[cutoff:] {
		recent[i] = true
	}

	out := make([]provider.Message, len(messages))
	copy(out, messages)
	for _, i := range toolIdx {
		name := names[out[i].ToolCallID]
		if name == "" {
			name = "result"
		}
		keep := recent[i] && newestByName[name] == i
		if keep {
			continue
		}
		out[i].Content = fmt.Sprintf("[tool %s: %d chars, truncated]", name, len(messages[i].Content))
	}
	return out
}

func toolCallNames(messages []provider.Message) map[string]string {
	out := make(map[string]string)
	for _, m := range messages {
		if m.Role != provider.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			out[tc.ID] = tc.Name
		}
	}
	return out
}
