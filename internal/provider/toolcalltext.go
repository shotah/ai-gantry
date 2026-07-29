package provider

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
)

// textCallSeq keeps synthesized tool-call ids unique within a process, since a
// tool result must echo the id of the call it answers.
var textCallSeq atomic.Uint64

// toolCallWrapper matches the packaging models put around a printed tool call:
// markdown fences and the <tool_call> tags some chat templates train on.
var toolCallWrapper = regexp.MustCompile("(?s)```[a-zA-Z]*|```|</?tool_call>|</?function_call>")

// ParseToolCallText recovers a tool call that the model wrote as text instead of
// emitting through the tool_calls field. Small local models do this routinely,
// and it is also the only channel available under a response_format grammar,
// which makes Ollama omit tool_calls entirely.
//
// Accepts the object bare, fenced, tagged, or embedded in prose, and reads
// arguments from whichever field name the model reached for ("parameters" is at
// least as common as "arguments").
func ParseToolCallText(content string) (ToolCall, bool) {
	stripped := toolCallWrapper.ReplaceAllString(content, "")
	obj, ok := firstJSONObject(stripped)
	if !ok {
		return ToolCall{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(obj), &fields); err != nil {
		return ToolCall{}, false
	}
	name := firstString(fields, "name", "tool", "tool_name", "function")
	if name == "" {
		return ToolCall{}, false
	}
	return ToolCall{
		ID:        fmt.Sprintf("call_text_%d", textCallSeq.Add(1)),
		Name:      name,
		Arguments: firstObject(fields, "arguments", "parameters", "args", "input"),
	}, true
}

// firstString returns the first key holding a non-empty JSON string.
func firstString(fields map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

// firstObject returns the first key holding a JSON object, accepting the
// stringified form OpenAI uses for arguments. Defaults to "{}" so a no-argument
// call still runs.
func firstObject(fields map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		if direct := strings.TrimSpace(string(raw)); strings.HasPrefix(direct, "{") {
			return direct
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if nested := strings.TrimSpace(s); strings.HasPrefix(nested, "{") {
				return nested
			}
		}
	}
	return "{}"
}

// firstJSONObject returns the first balanced {…} run, ignoring braces that occur
// inside string literals.
func firstJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		switch c := s[i]; {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}
