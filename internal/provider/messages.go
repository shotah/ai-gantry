package provider

import "strings"

// LLM_SYSTEM_FOLD modes: how RoleSystem blocks reach the OpenAI-compat body.
const (
	// FoldAuto folds for gemini* models, keeps the agent layout otherwise.
	FoldAuto = "auto"
	// FoldOne folds every system block into one leading system message
	// (Gemini; chat templates that render system only at position 0).
	FoldOne = "one"
	// FoldMany posts the agent layout as-is (trailing [harness] after the user).
	FoldMany = "many"
)

// WireMessages is the chat list actually posted on the OpenAI-compat body
// under FoldAuto. Gemini's compatibility layer keeps one system instruction:
// extra system turns (especially after a user) overwrite the persona or
// vanish, so this-turn [harness] clock/GPS never lands. Fold those into one
// leading system message. OpenAI and Ollama keep the agent's layout
// (trailing system after the user, prefix-cache friendly).
func WireMessages(model string, msgs []Message) []Message {
	return WireMessagesMode(FoldAuto, model, msgs)
}

// WireMessagesMode applies an explicit fold mode; empty or unknown is FoldAuto.
func WireMessagesMode(mode, model string, msgs []Message) []Message {
	if !foldToOne(mode, model) {
		return msgs
	}
	return foldSystemMessages(msgs)
}

func foldToOne(mode, model string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case FoldOne:
		return true
	case FoldMany:
		return false
	default:
		return geminiCompat(model)
	}
}

func geminiCompat(model string) bool {
	return strings.Contains(strings.ToLower(model), "gemini")
}

func foldSystemMessages(msgs []Message) []Message {
	lastUser := -1
	for i, m := range msgs {
		if m.Role == RoleUser {
			lastUser = i
		}
	}
	var turn, standing []string
	rest := make([]Message, 0, len(msgs))
	for i, m := range msgs {
		if m.Role != RoleSystem {
			rest = append(rest, m)
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		if lastUser >= 0 && i > lastUser {
			turn = append(turn, c)
		} else {
			standing = append(standing, c)
		}
	}
	if len(turn) == 0 && len(standing) == 0 {
		return rest
	}
	// Standing blocks (persona, summary, hydration) lead so identity is the
	// first thing read and the byte-stable prefix stays cacheable. This-turn
	// clock/GPS closes the one system blob for recency.
	blocks := append(append([]string{}, standing...), turn...)
	out := make([]Message, 0, 1+len(rest))
	out = append(out, Message{Role: RoleSystem, Content: strings.Join(blocks, "\n\n")})
	return append(out, rest...)
}
