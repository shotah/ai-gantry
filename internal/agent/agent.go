// Package agent implements the agent loop: prompt assembly, model calls, tool iteration, and reply.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
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
	Persona       string
	Completer     provider.Completer
	Sessions      History
	Tools         Tools         // optional
	Memory        memory.Memory // optional; hydration + persona precedence note
	Model         string
	MaxToolIters  int
	StreamReplies bool // stream final text via channel.ReplyWriter when Completer is a Streamer
	Logger        *slog.Logger
	StartedAt     time.Time
	// Location is the operator timezone for the per-turn temporal anchor (CRON_TZ).
	Location *time.Location
	TZName   string // IANA name for display (e.g. America/Los_Angeles)
}

// Agent runs the prompt → model → (tools) → reply loop.
type Agent struct {
	personaMu     sync.RWMutex
	persona       string
	completer     provider.Completer
	sessions      History
	tools         Tools
	memory        memory.Memory
	model         string
	maxToolIters  int
	streamReplies bool
	log           *slog.Logger
	startedAt     time.Time
	loc           *time.Location
	tzName        string
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
	loc := opts.Location
	if loc == nil {
		loc = time.Local
	}
	tzName := strings.TrimSpace(opts.TZName)
	if tzName == "" {
		tzName = loc.String()
	}
	a := &Agent{
		completer:     opts.Completer,
		sessions:      opts.Sessions,
		tools:         opts.Tools,
		memory:        opts.Memory,
		model:         opts.Model,
		maxToolIters:  maxIters,
		streamReplies: opts.StreamReplies,
		log:           log,
		startedAt:     started,
		loc:           loc,
		tzName:        tzName,
	}
	a.SetPersona(opts.Persona)
	return a, nil
}

// SetPersona replaces the system persona text (e.g. after SIGHUP reload).
// When memory is enabled, the persona-precedence note is appended.
func (a *Agent) SetPersona(text string) {
	if a.memory != nil {
		text = strings.TrimRight(text, "\n") + "\n" + strings.TrimSpace(memory.PersonaPrecedenceNote)
	}
	a.personaMu.Lock()
	a.persona = text
	a.personaMu.Unlock()
}

func (a *Agent) personaText() string {
	a.personaMu.RLock()
	defer a.personaMu.RUnlock()
	return a.persona
}

// Handle is a channel.Handler: assemble prompt, call model (with tools), return reply.
func (a *Agent) Handle(ctx context.Context, msg channel.Message) (string, error) {
	text := strings.TrimSpace(msg.Text)
	if text == "" && len(msg.Images) == 0 {
		return "", nil
	}
	storeText := text
	if storeText == "" {
		storeText = "[photo]"
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
		case "/tools":
			return a.listTools(), nil
		}
	}

	history, err := a.sessions.Messages(ctx, msg.SessionID)
	if err != nil {
		return "", err
	}

	messages := make([]provider.Message, 0, 3+len(history))
	if p := a.personaText(); p != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: p,
		})
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
	// Volatile per-turn blocks (hydration, clock) go AFTER history so the
	// stable prefix (persona + summary + history) stays byte-identical across
	// turns and llama.cpp/Ollama can reuse its prompt cache instead of
	// re-evaluating the whole context every message.
	hydrateQuery := text
	if hydrateQuery == "" {
		hydrateQuery = storeText
	}
	if a.memory != nil {
		entries, err := a.memory.Hydrate(ctx, hydrateQuery, 30)
		if err != nil {
			a.log.Warn("memory hydrate failed", "err", err)
		} else if block := memory.FormatHydration(entries); block != "" {
			messages = append(messages, provider.Message{
				Role:    provider.RoleSystem,
				Content: block,
			})
		}
	}
	// Fresh each turn (not stored in history) so "what time is it?" tracks reality.
	messages = append(messages, provider.Message{
		Role:    provider.RoleSystem,
		Content: temporalAnchor(time.Now().In(a.loc), a.tzName),
	})
	userMsg := provider.Message{
		Role:    provider.RoleUser,
		Content: storeText,
	}
	for _, img := range msg.Images {
		if u := strings.TrimSpace(img.URL); u != "" {
			userMsg.ImageURLs = append(userMsg.ImageURLs, u)
		}
	}
	messages = append(messages, userMsg)

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
		session.Message{Role: session.RoleUser, Content: storeText},
		session.Message{Role: session.RoleAssistant, Content: reply},
	); err != nil {
		return "", err
	}
	return reply, nil
}

func (a *Agent) runLoop(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (string, error) {
	streamer, canStream := a.completer.(provider.Streamer)
	writer, hasWriter := channel.ReplyWriterFrom(ctx)
	nudged := false
	sawTools := false

	for iter := 0; iter < a.maxToolIters; iter++ {
		bounded := collapseOldToolResults(messages)
		req := provider.Request{Messages: bounded, Tools: toolDefs}

		var (
			res *provider.Result
			err error
		)
		// Stream when enabled and a channel writer is present. Tool-call
		// responses still come back on the same stream path; onProgress is
		// skipped once tool deltas appear (see provider.CompleteStream).
		if a.streamReplies && canStream && hasWriter {
			tw, hasThinking := writer.(channel.ThinkingWriter)
			res, err = streamer.CompleteStream(ctx, req, func(content, thinking string) error {
				if hasThinking {
					return tw.UpdateThinking(ctx, thinking, content)
				}
				return writer.Update(ctx, content)
			})
		} else {
			res, err = a.completer.Complete(ctx, req)
		}
		if err != nil {
			return "", err
		}
		if res.FinishReason == "length" {
			a.log.Warn("model hit max_tokens (reply may be truncated)",
				"finish_reason", res.FinishReason,
				"chars", len(res.Content),
				"iteration", iter+1,
			)
		}
		if len(res.ToolCalls) == 0 {
			if res.Content == "" {
				if think := strings.TrimSpace(res.Thinking); think != "" {
					// CoT-only turns are common with Qwen think: the usable
					// answer lands in Thinking with empty Content. After tools
					// ran, promote CoT to the user reply — a nudge rarely
					// helps and burns another long think. Before tools, nudge
					// once; if still stuck, ERROR so Telegram reports it.
					a.log.Warn("model returned thinking with empty answer",
						"thinking_chars", len(res.Thinking),
						"finish_reason", res.FinishReason,
						"iteration", iter+1,
						"saw_tools", sawTools,
					)
					if sawTools {
						a.log.Info("promoting thinking to reply after tool results",
							"chars", len(think),
						)
						return think, nil
					}
					if !nudged {
						nudged = true
						messages = append(messages, provider.Message{
							Role: provider.RoleSystem,
							Content: "[system] Your previous response contained only internal reasoning — no visible reply and no tool call. " +
								"Act now: call the tool you decided on (use the exact tool name from the tools list), " +
								"or write your final answer as plain assistant text (not only inside thinking). Your reasoning was:\n" +
								clipChars(res.Thinking, 600),
						})
						continue
					}
					return "", fmt.Errorf("agent: model stalled after thinking (no reply, no tool call; thinking_chars=%d)", len(res.Thinking))
				}
				return "", fmt.Errorf("agent: empty model reply")
			}
			// Prose that promises a tool ("I'll pull…", mentions server__tool)
			// without actually emitting tool_calls — common Qwen failure. Nudge
			// once before tools; after tools, accept the text as the answer.
			if !sawTools && !nudged && promisesToolCall(res.Content, res.Thinking) {
				a.log.Warn("model promised tool call in prose without calling",
					"chars", len(res.Content),
					"iteration", iter+1,
				)
				nudged = true
				messages = append(messages, provider.Message{
					Role:    provider.RoleAssistant,
					Content: res.Content,
				})
				messages = append(messages, provider.Message{
					Role: provider.RoleSystem,
					Content: "[system] You described calling a tool but did not emit a tool call. " +
						"Do it now: invoke the exact tool name from the tools list (e.g. garmin__get_sleep for overnight sleep). " +
						"Do not narrate — call the function. For 'last night' sleep, omit date or pass today.",
				})
				continue
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
			sawTools = true
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

func (a *Agent) listTools() string {
	if a.tools == nil {
		return "tools: (none)"
	}
	defs := a.tools.Tools()
	if len(defs) == 0 {
		return "tools: (none)"
	}
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "tools (%d):\n", len(names))
	for _, name := range names {
		server, tool := splitPrefixedTool(name)
		if server != "" {
			fmt.Fprintf(&b, "- %s  (server=%s tool=%s)\n", name, server, tool)
		} else {
			fmt.Fprintf(&b, "- %s\n", name)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func splitPrefixedTool(name string) (server, tool string) {
	i := strings.Index(name, "__")
	if i <= 0 || i+2 >= len(name) {
		return "", name
	}
	return name[:i], name[i+2:]
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

// clipChars truncates s to at most n runes (with ellipsis when clipped).
func clipChars(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// promisesToolCall reports whether text talks about invoking a tool without
// having emitted tool_calls. Conservative cues only — real final answers that
// merely mention a past tool result should not match after sawTools.
func promisesToolCall(content, thinking string) bool {
	text := strings.ToLower(content + "\n" + thinking)
	if strings.TrimSpace(text) == "" {
		return false
	}
	if strings.Contains(text, "__") {
		return true
	}
	cues := []string{
		"let me pull", "let me call", "let me query", "let me fetch", "let me check",
		"i'll pull", "i'll call", "i'll query", "i'll fetch", "i'll check", "i'll use",
		"i will call", "i will pull", "i will query", "going to call", "about to call",
		"query body battery", "call this function", "calling the",
	}
	for _, cue := range cues {
		if strings.Contains(text, cue) {
			return true
		}
	}
	return false
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
