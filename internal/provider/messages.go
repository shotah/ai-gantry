package provider

import "strings"

// WireMessages is the chat list actually posted on the OpenAI-compat body.
// Gemini's compatibility layer keeps one system instruction: extra system
// turns (especially after a user) overwrite the persona or vanish, so
// this-turn [harness] clock/GPS never lands. Fold those into one leading
// system message. OpenAI and Ollama keep the agent's layout (trailing
// system after the user, prefix-cache friendly).
func WireMessages(model string, msgs []Message) []Message {
	if !geminiCompat(model) {
		return msgs
	}
	return foldGeminiMessages(msgs)
}

func geminiCompat(model string) bool {
	return strings.Contains(strings.ToLower(model), "gemini")
}

func foldGeminiMessages(msgs []Message) []Message {
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
	// This-turn clock/GPS first in the one system blob Gemini will keep.
	blocks := append(append([]string{}, turn...), standing...)
	out := make([]Message, 0, 1+len(rest))
	out = append(out, Message{Role: RoleSystem, Content: strings.Join(blocks, "\n\n")})
	return append(out, rest...)
}
