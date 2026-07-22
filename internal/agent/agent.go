// Package agent implements the agent loop: prompt assembly, model calls, and reply.
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

// History is the session-backed conversation store used by the agent.
type History interface {
	Messages(ctx context.Context, sessionID string) ([]session.Message, error)
	Append(ctx context.Context, sessionID string, msgs ...session.Message) error
	Reset(ctx context.Context, sessionID string) error
	Stats(ctx context.Context, sessionID string) (messages int, estTokens int, err error)
}

// Options configures a minimal (no-tools) agent.
type Options struct {
	Persona   string
	Completer provider.Completer
	Sessions  History
	Model     string
	Logger    *slog.Logger
	StartedAt time.Time
}

// Agent runs the prompt → model → reply loop.
type Agent struct {
	persona   string
	completer provider.Completer
	sessions  History
	model     string
	log       *slog.Logger
	startedAt time.Time
}

// New creates an Agent. Completer and Sessions are required.
func New(opts Options) (*Agent, error) {
	if opts.Completer == nil {
		return nil, fmt.Errorf("agent: Completer is required")
	}
	if opts.Sessions == nil {
		return nil, fmt.Errorf("agent: Sessions is required")
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	started := opts.StartedAt
	if started.IsZero() {
		started = time.Now()
	}
	return &Agent{
		persona:   opts.Persona,
		completer: opts.Completer,
		sessions:  opts.Sessions,
		model:     opts.Model,
		log:       log,
		startedAt: started,
	}, nil
}

// Handle is a channel.Handler: assemble prompt, call model, return reply.
func (a *Agent) Handle(ctx context.Context, msg channel.Message) (string, error) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return "", nil
	}

	if cmd, ok := parseCommand(text); ok {
		switch cmd {
		case "/new", "/clear":
			if err := a.sessions.Reset(ctx, msg.SessionID); err != nil {
				return "", err
			}
			return "session reset", nil
		case "/status":
			return a.status(ctx, msg.SessionID)
		}
	}

	history, err := a.sessions.Messages(ctx, msg.SessionID)
	if err != nil {
		return "", err
	}

	messages := make([]provider.Message, 0, 2+len(history))
	if a.persona != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: a.persona,
		})
	}
	for _, h := range history {
		messages = append(messages, provider.Message{
			Role:    provider.Role(h.Role),
			Content: h.Content,
		})
	}
	messages = append(messages, provider.Message{
		Role:    provider.RoleUser,
		Content: text,
	})

	a.log.Debug("agent complete",
		"session_id", msg.SessionID,
		"history_messages", len(history),
		"est_tokens", estTokens(messages),
	)

	reply, err := a.completer.Complete(ctx, messages)
	if err != nil {
		return "", err
	}

	if err := a.sessions.Append(ctx, msg.SessionID,
		session.Message{Role: session.RoleUser, Content: text},
		session.Message{Role: session.RoleAssistant, Content: reply},
	); err != nil {
		return "", err
	}
	return reply, nil
}

func (a *Agent) status(ctx context.Context, sessionID string) (string, error) {
	n, est, err := a.sessions.Stats(ctx, sessionID)
	if err != nil {
		return "", err
	}
	uptime := time.Since(a.startedAt).Truncate(time.Second)
	return fmt.Sprintf("uptime=%s model=%s history_messages=%d est_tokens=%d tools=0",
		uptime, a.model, n, est), nil
}

// parseCommand returns the slash command (lowercased, @bot suffix stripped)
// when the message is exactly that command (no args).
func parseCommand(text string) (string, bool) {
	fields := strings.Fields(text)
	if len(fields) != 1 {
		return "", false
	}
	cmd := fields[0]
	if !strings.HasPrefix(cmd, "/") {
		return "", false
	}
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	return strings.ToLower(cmd), true
}

func estTokens(messages []provider.Message) int {
	n := 0
	for _, m := range messages {
		n += (len(m.Content) + 3) / 4
	}
	return n
}
