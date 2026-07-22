// Package agent implements the agent loop: prompt assembly, model calls, tool iteration, and reply.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

// History is the session-backed conversation store used by the agent.
type History interface {
	Messages(ctx context.Context, sessionID string) ([]session.Message, error)
	Append(ctx context.Context, sessionID string, msgs ...session.Message) error
	Reset(ctx context.Context, sessionID string) error
	Stats(ctx context.Context, sessionID string) (messages int, estTokens int, err error)
	Summary(ctx context.Context, sessionID string) (string, error)
}

// Tools executes MCP (or other) tools during the agent loop.
type Tools interface {
	Tools() []provider.ToolDef
	Call(ctx context.Context, name string, arguments json.RawMessage) (string, error)
	ToolCount() int
}

// Options configures the agent.
type Options struct {
	Persona      string
	Completer    provider.Completer
	Sessions     History
	Tools        Tools         // optional
	Memory       memory.Memory // optional; hydration + persona precedence note
	Model        string
	MaxToolIters int
	Logger       *slog.Logger
	StartedAt    time.Time
}

// Agent runs the prompt → model → (tools) → reply loop.
type Agent struct {
	persona      string
	completer    provider.Completer
	sessions     History
	tools        Tools
	memory       memory.Memory
	model        string
	maxToolIters int
	log          *slog.Logger
	startedAt    time.Time
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
	maxIters := opts.MaxToolIters
	if maxIters < 1 {
		maxIters = 20
	}
	personaText := opts.Persona
	if opts.Memory != nil {
		personaText = strings.TrimRight(personaText, "\n") + "\n" + strings.TrimSpace(memory.PersonaPrecedenceNote)
	}
	return &Agent{
		persona:      personaText,
		completer:    opts.Completer,
		sessions:     opts.Sessions,
		tools:        opts.Tools,
		memory:       opts.Memory,
		model:        opts.Model,
		maxToolIters: maxIters,
		log:          log,
		startedAt:    started,
	}, nil
}

// Handle is a channel.Handler: assemble prompt, call model (with tools), return reply.
func (a *Agent) Handle(ctx context.Context, msg channel.Message) (string, error) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return "", nil
	}

	// Bind cron_* tools to this chat/session for scheduling.
	ctx = cron.WithDelivery(ctx, cron.Delivery{
		SessionID: msg.SessionID,
		UserID:    msg.UserID,
		ChatID:    msg.ChatID,
		ThreadID:  msg.ThreadID,
	})

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

	messages := make([]provider.Message, 0, 3+len(history))
	if a.persona != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: a.persona,
		})
	}
	if a.memory != nil {
		entries, err := a.memory.Hydrate(ctx, text, 30)
		if err != nil {
			a.log.Warn("memory hydrate failed", "err", err)
		} else if block := memory.FormatHydration(entries); block != "" {
			messages = append(messages, provider.Message{
				Role:    provider.RoleSystem,
				Content: block,
			})
		}
	}
	if summary, err := a.sessions.Summary(ctx, msg.SessionID); err != nil {
		a.log.Warn("session summary load failed", "err", err)
	} else if s := strings.TrimSpace(summary); s != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: "[session summary]\n" + s,
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

	var toolDefs []provider.ToolDef
	if a.tools != nil {
		toolDefs = a.tools.Tools()
	}

	a.log.Debug("agent complete",
		"session_id", msg.SessionID,
		"history_messages", len(history),
		"tools", len(toolDefs),
		"est_tokens", estTokens(messages),
	)

	reply, err := a.runLoop(ctx, messages, toolDefs)
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

func (a *Agent) runLoop(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (string, error) {
	for iter := 0; iter < a.maxToolIters; iter++ {
		bounded := collapseOldToolResults(messages)
		res, err := a.completer.Complete(ctx, provider.Request{
			Messages: bounded,
			Tools:    toolDefs,
		})
		if err != nil {
			return "", err
		}
		if len(res.ToolCalls) == 0 {
			if res.Content == "" {
				return "", fmt.Errorf("agent: empty model reply")
			}
			return res.Content, nil
		}
		if a.tools == nil {
			return "", fmt.Errorf("agent: model requested tools but none are configured")
		}

		messages = append(messages, provider.Message{
			Role:      provider.RoleAssistant,
			Content:   res.Content,
			ToolCalls: res.ToolCalls,
		})

		for _, call := range res.ToolCalls {
			a.log.Info("tool call",
				"name", call.Name,
				"id", call.ID,
				"iteration", iter+1,
			)
			args := json.RawMessage(call.Arguments)
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			out, err := a.tools.Call(ctx, call.Name, args)
			if err != nil {
				out = fmt.Sprintf("tool error: %v", err)
				a.log.Warn("tool call failed", "name", call.Name, "err", err)
			}
			messages = append(messages, provider.Message{
				Role:       provider.RoleTool,
				Content:    out,
				ToolCallID: call.ID,
			})
		}
	}
	return "", fmt.Errorf("agent: exceeded TOOL_MAX_ITERATIONS (%d)", a.maxToolIters)
}

func (a *Agent) status(ctx context.Context, sessionID string) (string, error) {
	n, est, err := a.sessions.Stats(ctx, sessionID)
	if err != nil {
		return "", err
	}
	tools := 0
	if a.tools != nil {
		tools = a.tools.ToolCount()
	}
	uptime := time.Since(a.startedAt).Truncate(time.Second)
	return fmt.Sprintf("uptime=%s model=%s history_messages=%d est_tokens=%d tools=%d",
		uptime, a.model, n, est, tools), nil
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
		for _, tc := range m.ToolCalls {
			n += (len(tc.Arguments) + 3) / 4
		}
	}
	return n
}
