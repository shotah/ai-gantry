package agent

import (
	"encoding/json"
	"fmt"

	"github.com/shotah/ai-gantry/internal/provider"
)

// keepRecentToolResults is how many trailing tool payloads stay in full.
// Older tool results collapse to a one-line marker (readme §5 bounding rules).
// Search-heavy MCPs (flights/rentals/cars) re-send results each iteration, so
// keeping 2 (not 4) cuts in-turn prefill without losing the latest evidence.
const keepRecentToolResults = 2

// collapsedToolArgs is the stub left on aging assistant tool-call arguments.
// Name and id stay so the provider can pair the collapsed result.
const collapsedToolArgs = "{}"

// collapseOldToolResults shortens tool payloads older than the recent window
// and stubs the matching assistant tool-call argument JSON. Within the window,
// older results that share a tool name with a newer one are also collapsed so
// repeated flights__offers_search calls do not stack.
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
	collapsedIDs := make(map[string]bool)
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
		if id := out[i].ToolCallID; id != "" {
			collapsedIDs[id] = true
		}
	}
	if len(collapsedIDs) > 0 {
		collapseOldToolCallArgs(out, collapsedIDs)
	}
	return out
}

// collapseOldToolCallArgs stubs argument JSON on assistant tool calls whose
// results were collapsed. Copies the ToolCalls slice so the caller's messages
// are not mutated. Gemini thought_signature in Raw is kept; only arguments shrink.
func collapseOldToolCallArgs(messages []provider.Message, collapsedIDs map[string]bool) {
	for i, m := range messages {
		if m.Role != provider.RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		copied := false
		for j, tc := range m.ToolCalls {
			if !collapsedIDs[tc.ID] {
				continue
			}
			if !copied {
				calls := make([]provider.ToolCall, len(m.ToolCalls))
				copy(calls, m.ToolCalls)
				messages[i].ToolCalls = calls
				copied = true
			}
			messages[i].ToolCalls[j] = stubToolCallArgs(messages[i].ToolCalls[j])
		}
	}
}

func stubToolCallArgs(tc provider.ToolCall) provider.ToolCall {
	tc.Arguments = collapsedToolArgs
	if len(tc.Raw) == 0 {
		return tc
	}
	var payload map[string]any
	if err := json.Unmarshal(tc.Raw, &payload); err != nil {
		tc.Raw = nil
		return tc
	}
	if fn, ok := payload["function"].(map[string]any); ok {
		fn["arguments"] = collapsedToolArgs
	}
	b, err := json.Marshal(payload)
	if err != nil {
		tc.Raw = nil
		return tc
	}
	tc.Raw = b
	return tc
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
