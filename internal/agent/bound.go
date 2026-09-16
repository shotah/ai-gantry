package agent

import (
	"encoding/json"
	"fmt"

	"github.com/shotah/ai-gantry/internal/provider"
)

// keepRecentToolRounds is how many trailing tool rounds — one model-emitted
// batch each — keep their payloads in full. Older rounds collapse to a
// one-line marker (readme §5 bounding rules). The unit is the round, not the
// payload: a model that asks three things at once — two flight dates and the
// calendar, as the persona's "prefer parallel tool calls" invites — has to
// see all three answers next round. Counting payloads (and squeezing
// same-name results to the newest) hid the first one behind a "[truncated]"
// stub before it was ever read, and the model did the only sensible things:
// searched again (a paid call, paid twice) or filled the hole in with a fare
// no tool returned. Search-heavy MCPs (flights/rentals/cars) re-send results
// each iteration, so 2 rounds (not 4) still bounds in-turn prefill.
const keepRecentToolRounds = 2

// collapsedToolArgs is the stub left on aging assistant tool-call arguments.
// Name and id stay so the provider can pair the collapsed result.
const collapsedToolArgs = "{}"

// collapseOldToolResults shortens tool payloads from rounds older than the
// recent window and stubs the matching assistant tool-call argument JSON.
// Everything in the last keepRecentToolRounds rounds stays whole, however
// many calls a round made and whatever they were named.
func collapseOldToolResults(messages []provider.Message) []provider.Message {
	roundOf := make(map[string]int)
	rounds := 0
	for _, m := range messages {
		if m.Role != provider.RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			roundOf[tc.ID] = rounds
		}
		rounds++
	}
	if rounds <= keepRecentToolRounds {
		return messages
	}
	oldestKept := rounds - keepRecentToolRounds
	names := toolCallNames(messages)

	out := make([]provider.Message, len(messages))
	copy(out, messages)
	collapsedIDs := make(map[string]bool)
	seen := 0 // rounds started before this position — the round of an unpaired result
	for i, m := range messages {
		if m.Role == provider.RoleAssistant && len(m.ToolCalls) > 0 {
			seen++
			continue
		}
		if m.Role != provider.RoleTool {
			continue
		}
		round, ok := roundOf[m.ToolCallID]
		if !ok {
			round = seen - 1
		}
		if round >= oldestKept {
			continue
		}
		name := names[m.ToolCallID]
		if name == "" {
			name = "result"
		}
		out[i].Content = fmt.Sprintf("[tool %s: %d chars, truncated]", name, len(m.Content))
		if m.ToolCallID != "" {
			collapsedIDs[m.ToolCallID] = true
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
