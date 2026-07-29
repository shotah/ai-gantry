package provider

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
)

// textCallSeq keeps synthesized tool-call ids unique within a process, since a
// tool result must echo the id of the call it answers.
var textCallSeq atomic.Uint64

// toolCallWrapper matches the packaging models put around a printed tool call:
// markdown fences and the <tool_call> tags some chat templates train on.
var toolCallWrapper = regexp.MustCompile("(?s)```[a-zA-Z]*|```|</?tool_call>|</?function_call>")

// prefixedToolName matches server__tool (MCP host naming).
var prefixedToolName = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*__[A-Za-z0-9_]+`)

// ParseToolCallText recovers a tool call that the model wrote as text instead of
// emitting through the tool_calls field. Small local models do this routinely,
// and it is also the only channel available under a response_format grammar,
// which makes Ollama omit tool_calls entirely.
//
// Accepts:
//   - JSON objects (bare / fenced / tagged / embedded), with arguments under
//     "parameters" / "arguments" / …
//   - Function-call prose Qwen often invents:
//     {google__calendar_list_events(time_min="…", calendar_id="primary")}
//   - Args-only JSON (```json {…} ```) paired with a server__tool name in the
//     same text, or a single hint name from the prior assistant turn
func ParseToolCallText(content string) (ToolCall, bool) {
	return ParseToolCallTextHinted(content, nil)
}

// ParseToolCallTextHinted is ParseToolCallText plus optional tool-name hints
// (e.g. the previous assistant message printed the name, this turn only the args).
func ParseToolCallTextHinted(content string, hintNames []string) (ToolCall, bool) {
	stripped := toolCallWrapper.ReplaceAllString(content, "")
	if call, ok := parseJSONToolCall(stripped); ok {
		return call, true
	}
	if call, ok := parseFnStyleToolCall(stripped); ok {
		return call, true
	}
	return parseArgsOnlyToolCall(stripped, hintNames)
}

// PrefixedToolNames returns unique server__tool identifiers found in s.
func PrefixedToolNames(s string) []string {
	raw := prefixedToolName.FindAllString(s, -1)
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, n := range raw {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func parseJSONToolCall(content string) (ToolCall, bool) {
	obj, ok := firstJSONObject(content)
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

// parseArgsOnlyToolCall recovers a fenced/bare JSON object of arguments when the
// tool name is nearby (or uniquely hinted), e.g.:
//
//	```json
//	{"calendar_id":"primary","time_min":"…"}
//	```
func parseArgsOnlyToolCall(content string, hintNames []string) (ToolCall, bool) {
	obj, ok := firstJSONObject(content)
	if !ok {
		return ToolCall{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(obj), &fields); err != nil || len(fields) == 0 {
		return ToolCall{}, false
	}
	// Named wrappers belong to parseJSONToolCall (failed ⇒ empty name ⇒ reject).
	if firstString(fields, "name", "tool", "tool_name", "function") != "" {
		return ToolCall{}, false
	}
	args := obj
	if inner := firstObject(fields, "arguments", "parameters", "args", "input"); inner != "{}" {
		// {"parameters":{…}} without a name — use the inner object.
		onlyWrapper := true
		for k := range fields {
			switch k {
			case "arguments", "parameters", "args", "input":
			default:
				onlyWrapper = false
			}
		}
		if onlyWrapper {
			args = inner
		}
	}

	name := ""
	inContent := PrefixedToolNames(content)
	switch {
	case len(inContent) == 1:
		name = inContent[0]
	case len(inContent) == 0:
		if hints := uniqueNonEmpty(hintNames); len(hints) == 1 {
			name = hints[0]
		}
	}
	if name == "" || !strings.Contains(name, "__") {
		return ToolCall{}, false
	}
	if !mostlyToolArgDump(content, obj, name) {
		return ToolCall{}, false
	}
	return ToolCall{
		ID:        fmt.Sprintf("call_text_%d", textCallSeq.Add(1)),
		Name:      name,
		Arguments: args,
	}, true
}

func uniqueNonEmpty(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// mostlyToolArgDump requires the message to be little more than the tool name
// plus the JSON args (brat prints), not a real answer that happens to mention a tool.
func mostlyToolArgDump(content, obj, name string) bool {
	rest := content
	rest = strings.Replace(rest, obj, "", 1)
	rest = strings.ReplaceAll(rest, name, "")
	rest = toolCallWrapper.ReplaceAllString(rest, "")
	rest = strings.TrimSpace(rest)
	letters, words := 0, 0
	inWord := false
	for _, r := range rest {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letters++
			if !inWord {
				words++
				inWord = true
			}
			continue
		}
		inWord = false
	}
	// Allow tiny glue ("Calling", "OK") — not "I already used … earlier".
	return letters <= 24 && words <= 3
}

// parseFnStyleToolCall recovers {server__tool(k="v", …)} / server__tool(…).
func parseFnStyleToolCall(content string) (ToolCall, bool) {
	loc := prefixedToolName.FindStringIndex(content)
	if loc == nil {
		return ToolCall{}, false
	}
	name := content[loc[0]:loc[1]]
	rest := strings.TrimSpace(content[loc[1]:])
	if !strings.HasPrefix(rest, "(") {
		return ToolCall{}, false
	}
	inside, ok := balancedParens(rest[1:])
	if !ok {
		return ToolCall{}, false
	}
	args, ok := parseKwArgs(inside)
	if !ok {
		return ToolCall{}, false
	}
	return ToolCall{
		ID:        fmt.Sprintf("call_text_%d", textCallSeq.Add(1)),
		Name:      name,
		Arguments: args,
	}, true
}

// balancedParens returns the text before the matching ')' for s that begins
// inside an already-opened '('.
func balancedParens(s string) (string, bool) {
	depth := 1
	inString := false
	quote := byte(0)
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case inString && c == quote:
			inString = false
		case inString:
		case c == '"' || c == '\'':
			inString = true
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return s[:i], true
			}
		}
	}
	return "", false
}

// parseKwArgs turns key="value", key=123 into a JSON object.
func parseKwArgs(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "{}", true
	}
	args := map[string]any{}
	i := 0
	for i < len(s) {
		for i < len(s) && (unicode.IsSpace(rune(s[i])) || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			break
		}
		keyStart := i
		for i < len(s) && (unicode.IsLetter(rune(s[i])) || unicode.IsDigit(rune(s[i])) || s[i] == '_') {
			i++
		}
		if i == keyStart {
			return "", false
		}
		key := s[keyStart:i]
		for i < len(s) && unicode.IsSpace(rune(s[i])) {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			return "", false
		}
		i++
		for i < len(s) && unicode.IsSpace(rune(s[i])) {
			i++
		}
		if i >= len(s) {
			return "", false
		}
		val, next, ok := readKwValue(s, i)
		if !ok {
			return "", false
		}
		args[key] = val
		i = next
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func readKwValue(s string, i int) (any, int, bool) {
	switch s[i] {
	case '"', '\'':
		q := s[i]
		i++
		start := i
		escaped := false
		for i < len(s) {
			c := s[i]
			if escaped {
				escaped = false
				i++
				continue
			}
			if c == '\\' {
				escaped = true
				i++
				continue
			}
			if c == q {
				raw := s[start:i]
				return unescapeSimple(raw), i + 1, true
			}
			i++
		}
		return nil, 0, false
	default:
		start := i
		for i < len(s) && s[i] != ',' && !unicode.IsSpace(rune(s[i])) {
			i++
		}
		tok := s[start:i]
		switch strings.ToLower(tok) {
		case "true":
			return true, i, true
		case "false":
			return false, i, true
		case "null":
			return nil, i, true
		}
		if n, err := strconv.ParseInt(tok, 10, 64); err == nil {
			return n, i, true
		}
		if n, err := strconv.ParseFloat(tok, 64); err == nil {
			return n, i, true
		}
		// Bare identifier — keep as string (e.g. primary without quotes).
		if tok != "" && (unicode.IsLetter(rune(tok[0])) || tok[0] == '_') {
			return tok, i, true
		}
		return nil, 0, false
	}
}

func unescapeSimple(s string) string {
	// Enough for typical tool args; full JSON unescape is unnecessary.
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\'`, `'`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
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
