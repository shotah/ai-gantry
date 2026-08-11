package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

// SelfNotes persists the agent's self-maintained personality file (SELF.md).
// Implemented by selfnote.Store; writes trigger a persona reload via its hook.
type SelfNotes interface {
	Read() (string, error)
	Write(content string) error
}

// selfDistillMinMessages is the smallest session worth distilling — below
// this a reset has not built any personality worth keeping.
const selfDistillMinMessages = 6

// selfDistillTimeout bounds the extra model call on /new. Reset must never
// hang behind a dead provider — /new is often how users escape one.
const selfDistillTimeout = 2 * time.Minute

// Transcript bounds for the distill prompt: enough to catch running jokes,
// small enough that a local model prefills it quickly.
const (
	selfDistillMaxMessages = 60
	selfDistillPerMessage  = 400
)

const selfDistillPrompt = "[system] This chat session is about to be reset and its history erased. " +
	"Below are your current self-notes (SELF.md) and the dying conversation. " +
	"Rewrite the complete self-notes file so the personality you developed here survives: " +
	"keep existing notes that still matter and fold in voice, humor, running jokes, games, nicknames, and rituals from this conversation worth keeping. " +
	"Rules: output only the file content; start with the heading \"# SELF.md — Who You Are Becoming\"; " +
	"short \"- \" bullet lines, at most 30; notes describe YOUR personality and shared rituals — " +
	"not facts about the human, not rules, not tool recipes."

// distillSelf folds the dying session's personality into SELF.md before a
// reset. Best-effort: every failure is logged and the reset proceeds — losing
// a distill must never block /new.
func (a *Agent) distillSelf(ctx context.Context, sessionID string) bool {
	history, err := a.sessions.Messages(ctx, sessionID)
	if err != nil {
		a.log.Warn("self distill: history load failed", "err", err)
		return false
	}
	if len(history) < selfDistillMinMessages {
		return false
	}
	current, err := a.selfNotes.Read()
	if err != nil {
		a.log.Warn("self distill: read failed", "err", err)
		return false
	}
	if current == "" {
		current = "(none yet)"
	}

	dctx, cancel := context.WithTimeout(ctx, selfDistillTimeout)
	defer cancel()
	res, err := a.completer.Complete(dctx, provider.Request{Messages: []provider.Message{
		{Role: provider.RoleSystem, Content: selfDistillPrompt},
		{Role: provider.RoleSystem, Content: "[current SELF.md]\n" + current},
		{Role: provider.RoleUser, Content: "[transcript]\n" + distillTranscript(history)},
	}})
	if err != nil {
		a.log.Warn("self distill: model call failed", "err", err)
		return false
	}
	content := stripCodeFence(strings.TrimSpace(res.Content))
	if content == "" {
		a.log.Warn("self distill: empty model reply", "thinking_chars", len(res.Thinking))
		return false
	}
	if err := a.selfNotes.Write(content); err != nil {
		a.log.Warn("self distill: write failed", "err", err)
		return false
	}
	a.log.Info("self distill: SELF.md updated", "chars", len(content), "history_messages", len(history))
	return true
}

// distillTranscript renders bounded recent history for the distill prompt.
func distillTranscript(history []session.Message) string {
	if len(history) > selfDistillMaxMessages {
		history = history[len(history)-selfDistillMaxMessages:]
	}
	var b strings.Builder
	for _, m := range history {
		fmt.Fprintf(&b, "%s: %s\n", m.Role, clipChars(m.Content, selfDistillPerMessage))
	}
	return b.String()
}

// stripCodeFence unwraps a reply a chatty model wrapped in ``` fences.
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimSpace(strings.TrimSuffix(s, "```"))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return ""
}
