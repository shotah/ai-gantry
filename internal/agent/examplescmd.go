package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/examples"
	"github.com/shotah/ai-gantry/internal/provider"
)

// ExamplesControl is the optional training-wheels surface for /examples.
type ExamplesControl interface {
	SuggestPrompt() (prompt string, seed examples.Seed, ok bool)
	SetEnabled(ctx context.Context, sessionID string, on bool) error
	EnsureFor(ctx context.Context, delivery cron.Delivery) (cron.Job, bool, error)
	ProactiveEnabled() bool
}

// parseExamplesCommand recognizes /examples with optional on|off|true|false.
func parseExamplesCommand(text string) (arg string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", false
	}
	cmd := fields[0]
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	if !strings.EqualFold(cmd, "/examples") {
		return "", false
	}
	if len(fields) >= 2 {
		arg = strings.ToLower(strings.TrimSpace(fields[1]))
	}
	return arg, true
}

func (a *Agent) handleExamples(ctx context.Context, msg channelDelivery, arg string) (string, error) {
	if a.examples == nil {
		return "examples: not configured (cron/store unavailable)", nil
	}
	switch arg {
	case "":
		return a.suggestExample(ctx)
	case "on", "true":
		if err := a.examples.SetEnabled(ctx, msg.SessionID, true); err != nil {
			return "", err
		}
		if !a.examples.ProactiveEnabled() {
			return "examples on for this chat — proactive pings are disabled by EXAMPLES_QTY; /examples still works anytime", nil
		}
		_, _, err := a.examples.EnsureFor(ctx, cron.Delivery{
			SessionID: msg.SessionID,
			UserID:    msg.UserID,
			ChatID:    msg.ChatID,
			ThreadID:  msg.ThreadID,
		})
		if err != nil {
			return "", err
		}
		return "examples on — you'll get occasional capability ideas (turn off with /examples off)", nil
	case "off", "false":
		if err := a.examples.SetEnabled(ctx, msg.SessionID, false); err != nil {
			return "", err
		}
		return "examples off — no more proactive ideas; /examples still works anytime to request one", nil
	default:
		return "usage: /examples | /examples on | /examples off", nil
	}
}

// channelDelivery is the subset of channel.Message needed for examples ensure.
type channelDelivery struct {
	SessionID string
	UserID    string
	ChatID    string
	ThreadID  int
}

func (a *Agent) suggestExample(ctx context.Context) (string, error) {
	prompt, seed, ok := a.examples.SuggestPrompt()
	if !ok {
		return examples.NoMatchMessage, nil
	}
	reply, err := a.polishExample(ctx, prompt)
	if err != nil || strings.TrimSpace(reply) == "" {
		a.log.Warn("examples polish failed; using fallback", "err", err, "seed", seed.ID)
		return examples.FallbackFormat(seed), nil
	}
	if !strings.Contains(strings.ToLower(reply), "/examples off") {
		reply = strings.TrimSpace(reply) + "\n\n" + examples.OffHint
	}
	return reply, nil
}

func (a *Agent) polishExample(ctx context.Context, prompt string) (string, error) {
	req := provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: a.personaText()},
			{Role: provider.RoleUser, Content: prompt},
		},
	}
	res, err := a.completer.Complete(ctx, req)
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", fmt.Errorf("empty completion")
	}
	return strings.TrimSpace(res.Content), nil
}
