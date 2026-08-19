package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/selfnote"
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
	"Below are your current self-notes (SELF.md), the dying conversation, and (if present) a " +
	"[session voice] block — the rolling mood of this chat (jokes, nicknames, games). " +
	"Merge into a complete self-notes file so the personality you developed here survives. " +
	"Keep every existing SELF.md bullet that is a joke, nickname, game, ritual, or standing aim unless it is a true duplicate. " +
	"Add from [session voice] using exact wording. Do not replace a quoted joke with a mood word. " +
	"A standing aim outlives one task; keep those. Do not copy one-off to-dos or Facts: about the human. " +
	"Prefer exact wording from [session voice] over paraphrasing the transcript. " +
	"Skip one-off mood weather (\"dry today\"). Do not copy facts about the human into SELF.md. " +
	"Rules: output only the file content; start with the heading \"# SELF.md — Who You Are Becoming\"; " +
	"short \"- \" bullet lines, at most 30; notes describe YOUR personality, shared rituals, and standing aims — " +
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
	_, voice := a.sessionLedger(ctx, sessionID)
	if len(history) < selfDistillMinMessages && voice == "" {
		return false
	}
	if voice == "" && !transcriptHasQuotedSpan(history) {
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

	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: selfDistillPrompt},
		{Role: provider.RoleSystem, Content: "[current SELF.md]\n" + current},
	}
	if voice != "" {
		msgs = append(msgs, provider.Message{
			Role:    provider.RoleSystem,
			Content: "[session voice]\n" + voice,
		})
	}
	msgs = append(msgs, provider.Message{
		Role:    provider.RoleUser,
		Content: "[transcript]\n" + distillTranscript(history),
	})

	dctx, cancel := context.WithTimeout(ctx, selfDistillTimeout)
	defer cancel()
	res, err := a.completer.Complete(dctx, provider.Request{Messages: msgs})
	if err != nil {
		a.log.Warn("self distill: model call failed", "err", err)
		return false
	}
	content := stripCodeFence(strings.TrimSpace(res.Content))
	if content == "" {
		a.log.Warn("self distill: empty model reply", "thinking_chars", len(res.Thinking))
		return false
	}
	if current != "" && current != "(none yet)" {
		content = selfnote.RestoreQuotedLines(current, content)
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

func transcriptHasQuotedSpan(history []session.Message) bool {
	for _, m := range history {
		if hasQuotedSpan(m.Content) {
			return true
		}
	}
	return false
}

func hasQuotedSpan(s string) bool {
	i := strings.IndexByte(s, '"')
	if i < 0 {
		return false
	}
	return strings.IndexByte(s[i+1:], '"') >= 0
}

// parkSessionFacts writes the dying session's Facts: block into SQLite as one
// episode so the consolidator can split it into durable rows. USER.md is
// operator-owned and is never written here.
func (a *Agent) parkSessionFacts(ctx context.Context, sessionID string) bool {
	if a.memory == nil {
		return false
	}
	facts, _ := a.sessionLedger(ctx, sessionID)
	if facts == "" {
		return false
	}
	if _, err := a.memory.Store(ctx, memory.KindEpisode, "session", facts); err != nil {
		a.log.Warn("session facts: park failed", "err", err)
		return false
	}
	a.log.Info("session facts: parked in memory", "chars", len(facts))
	return true
}

func (a *Agent) sessionLedger(ctx context.Context, sessionID string) (facts, voice string) {
	if a.sessions == nil {
		return "", ""
	}
	summary, err := a.sessions.Summary(ctx, sessionID)
	if err != nil {
		a.log.Warn("session ledger: summary load failed", "err", err)
		return "", ""
	}
	return session.LedgerParts(summary)
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
