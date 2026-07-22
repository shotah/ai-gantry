// Package agent implements the agent loop: prompt assembly, model calls, and reply.
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

// Options configures a minimal (no-tools) agent.
type Options struct {
	Persona      string
	Completer    provider.Completer
	Model        string
	MaxMessages  int // HISTORY_MAX_MESSAGES
	MaxEstTokens int // HISTORY_MAX_TOKENS (chars/4 estimate)
	Logger       *slog.Logger
	StartedAt    time.Time
}

// Agent runs the prompt → model → reply loop with in-memory session history.
type Agent struct {
	persona      string
	completer    provider.Completer
	model        string
	maxMessages  int
	maxEstTokens int
	log          *slog.Logger
	startedAt    time.Time

	mu       sync.Mutex
	sessions map[string][]provider.Message
}

// New creates an Agent. Completer is required.
func New(opts Options) (*Agent, error) {
	if opts.Completer == nil {
		return nil, fmt.Errorf("agent: Completer is required")
	}
	maxMessages := opts.MaxMessages
	if maxMessages < 1 {
		maxMessages = 200
	}
	maxEstTokens := opts.MaxEstTokens
	if maxEstTokens < 1 {
		maxEstTokens = 128000
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
		persona:      opts.Persona,
		completer:    opts.Completer,
		model:        opts.Model,
		maxMessages:  maxMessages,
		maxEstTokens: maxEstTokens,
		log:          log,
		startedAt:    started,
		sessions:     make(map[string][]provider.Message),
	}, nil
}

// Handle is a channel.Handler: assemble prompt, call model, return reply.
func (a *Agent) Handle(ctx context.Context, msg channel.Message) (string, error) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return "", nil
	}

	switch strings.ToLower(text) {
	case "/new":
		a.reset(msg.SessionID)
		return "session reset", nil
	case "/status":
		return a.status(msg.SessionID), nil
	}

	a.mu.Lock()
	history := append([]provider.Message(nil), a.sessions[msg.SessionID]...)
	a.mu.Unlock()

	messages := make([]provider.Message, 0, 2+len(history))
	if a.persona != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: a.persona,
		})
	}
	messages = append(messages, history...)
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

	a.append(msg.SessionID, provider.Message{Role: provider.RoleUser, Content: text}, provider.Message{
		Role:    provider.RoleAssistant,
		Content: reply,
	})
	return reply, nil
}

func (a *Agent) reset(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, sessionID)
}

func (a *Agent) append(sessionID string, turns ...provider.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	hist := append(a.sessions[sessionID], turns...)
	a.sessions[sessionID] = trimHistory(hist, a.maxMessages, a.maxEstTokens)
}

func (a *Agent) status(sessionID string) string {
	a.mu.Lock()
	n := len(a.sessions[sessionID])
	est := estTokens(a.sessions[sessionID])
	a.mu.Unlock()
	uptime := time.Since(a.startedAt).Truncate(time.Second)
	return fmt.Sprintf("uptime=%s model=%s history_messages=%d est_tokens=%d tools=0",
		uptime, a.model, n, est)
}

// trimHistory drops oldest turns past message/token caps.
// Persona is not stored here; last turns are kept when possible.
func trimHistory(hist []provider.Message, maxMessages, maxEstTokens int) []provider.Message {
	for len(hist) > maxMessages {
		hist = hist[1:]
	}
	for estTokens(hist) > maxEstTokens && len(hist) > 2 {
		hist = hist[1:]
	}
	return hist
}

func estTokens(messages []provider.Message) int {
	n := 0
	for _, m := range messages {
		n += (len(m.Content) + 3) / 4 // chars/4 estimate
	}
	return n
}
