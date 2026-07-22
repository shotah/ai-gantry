package agent

import (
	"fmt"

	"github.com/shotah/ai-gantry/internal/provider"
)

// keepRecentToolResults is how many trailing tool payloads stay in full.
// Older tool results collapse to a one-line marker (readme §6 bounding rules).
const keepRecentToolResults = 4

// collapseOldToolResults shortens tool payloads older than the recent window.
func collapseOldToolResults(messages []provider.Message) []provider.Message {
	var toolIdx []int
	for i, m := range messages {
		if m.Role == provider.RoleTool {
			toolIdx = append(toolIdx, i)
		}
	}
	if len(toolIdx) <= keepRecentToolResults {
		return messages
	}
	names := toolCallNames(messages)
	cutoff := len(toolIdx) - keepRecentToolResults
	out := make([]provider.Message, len(messages))
	copy(out, messages)
	for _, i := range toolIdx[:cutoff] {
		name := names[out[i].ToolCallID]
		if name == "" {
			name = "result"
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
