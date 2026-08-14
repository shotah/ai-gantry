// Package agent implements the agent loop: prompt assembly, model calls, tool iteration, and reply.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/mcp"
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

// Tool-trace modes for user-visible progress (see TOOL_TRACE).
const (
	ToolTraceFull    = "full"    // → name / ✓ timing · chars
	ToolTraceCompact = "compact" // Making Calls: ✓, ✗, ✓ (default)
	ToolTraceOff     = "off"     // hide tool activity
)

// compactCallsHeader is the progress line opened in TOOL_TRACE=compact.
const compactCallsHeader = "Making Calls:"

// budgetExhaustedNote is injected on the landing call, where no tools are
// offered, so the model reports what it has instead of erroring out.
const budgetExhaustedNote = "[system] Tool budget exhausted: all %d tool rounds for this turn are used and no more tool calls are possible. " +
	"Write your final reply to the user now: summarize what you accomplished, what you found, and what is still unfinished."

// toolNarrationNote is appended to the system persona when tools are wired so
// the visible trace carries a "why" line ahead of each call's ✓/✗ marks.
const toolNarrationNote = `Before each tool call, write one short line of visible reply text saying what you are doing and why (e.g. "Searching contacts for Joe"). Keep it under a dozen words and make the call in the same response.`

// cronToolFirstNote sits after the clock on scheduled turns so the last
// instruction is "tools first" — cron user text otherwise reads like a
// finished-report spec and small models draft numbers instead of calling.
const cronToolFirstNote = "[system] Scheduled turn: if this job needs live data, emit tool calls now and wait for results. Do not invent metrics, events, or search results. Write the user-facing report only after tool results are in context. If no tools are needed, reply now."

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
	// ToolTrace is full|compact|off. Empty defaults to compact.
	ToolTrace string
	Logger    *slog.Logger
	StartedAt time.Time
	// Location is the operator timezone for the per-turn temporal anchor (CRON_TZ).
	Location *time.Location
	TZName   string // IANA name for display (e.g. America/Los_Angeles)
	// CoalesceSettle waits this long after the last bubble before running a
	// joined turn (interrupt + coalesce). 0 disables. Production default is
	// DefaultCoalesceSettle via run config.
	CoalesceSettle time.Duration
	// SpinupNotice posts a "still working" line once a turn has gone this long
	// without model output. The first turn of the process posts immediately
	// instead of waiting. 0 disables both notices.
	SpinupNotice time.Duration
	// SelfNotes is optional; when set, /new distills the dying session's
	// personality into SELF.md before the reset (see internal/selfnote).
	SelfNotes SelfNotes
	// Consolidator is optional; /memstats reads last_run from it when set.
	Consolidator *memory.Consolidator
	// MCPManifest is the path to mcp.toml for /auth (chat OAuth). Empty disables /auth.
	MCPManifest string
	// Examples is optional; enables /examples (instant + on/off for proactive pings).
	Examples ExamplesControl
}

// Agent runs the prompt → model → (tools) → reply loop.
type Agent struct {
	personaMu     sync.RWMutex
	persona       string
	completer     provider.Completer
	sessions      History
	tools         Tools
	memory        memory.Memory
	selfNotes     SelfNotes
	model         string
	maxToolIters  int
	streamReplies bool
	toolTrace     string
	log           *slog.Logger
	startedAt     time.Time
	loc           *time.Location
	tzName        string

	turnMu       sync.Mutex
	turnSeq      uint64
	turns        map[string]*turnSlot // sessionID → in-flight turn
	sessionMu    sync.Mutex
	sessionLocks map[string]*sessionGate

	coalesceSettle time.Duration
	coalesceMu     sync.Mutex
	coalesce       map[string]*coalesceSession

	spinupNotice time.Duration
	warmed       atomic.Bool // set once any model call has returned

	perf *perfRing

	// consolidator is optional; used by /memstats for last_run (builtin only).
	consolidator *memory.Consolidator

	mcpManifest string
	examples    ExamplesControl
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
		maxIters = 10
	}
	toolTrace := strings.ToLower(strings.TrimSpace(opts.ToolTrace))
	switch toolTrace {
	case ToolTraceFull, ToolTraceCompact, ToolTraceOff:
	default:
		toolTrace = ToolTraceCompact
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
		completer:      opts.Completer,
		sessions:       opts.Sessions,
		tools:          opts.Tools,
		memory:         opts.Memory,
		selfNotes:      opts.SelfNotes,
		model:          opts.Model,
		maxToolIters:   maxIters,
		streamReplies:  opts.StreamReplies,
		toolTrace:      toolTrace,
		log:            log,
		startedAt:      started,
		loc:            loc,
		tzName:         tzName,
		coalesceSettle: opts.CoalesceSettle,
		spinupNotice:   opts.SpinupNotice,
		consolidator:   opts.Consolidator,
		mcpManifest:    strings.TrimSpace(opts.MCPManifest),
		examples:       opts.Examples,
		perf:           newPerfRing(started),
	}
	a.initTurns()
	a.SetPersona(opts.Persona)
	return a, nil
}

// SetPersona replaces the system persona text (e.g. after SIGHUP reload).
// When tools are wired, the narration note is appended; when memory is
// enabled, the persona-precedence note is appended.
func (a *Agent) SetPersona(text string) {
	if a.tools != nil {
		text = strings.TrimRight(text, "\n") + "\n" + toolNarrationNote
	}
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

	// Bind cron_* tools to this chat/session for scheduling.
	ctx = cron.WithDelivery(ctx, cron.Delivery{
		SessionID: msg.SessionID,
		UserID:    msg.UserID,
		ChatID:    msg.ChatID,
		ThreadID:  msg.ThreadID,
	})

	// /cancel must not take the session lock — it runs on a parallel Telegram
	// worker while the in-flight turn still holds that lock.
	if cmd, ok := parseCommand(text); ok && (cmd == "/cancel" || cmd == "/stop") {
		a.coalesceClear(msg.SessionID)
		if a.Cancel(msg.SessionID) {
			return "cancelled — stopped the in-flight turn (tools that already finished are not undone)", nil
		}
		return "nothing in progress to cancel", nil
	}

	// /auth accepts args (/auth strava <code>) so it cannot use parseCommand.
	if server, arg, ok := parseAuthCommand(text); ok {
		unlock := a.lockSession(msg.SessionID)
		defer unlock()
		return a.handleAuth(ctx, server, arg)
	}

	// /examples accepts on|off|true|false.
	if arg, ok := parseExamplesCommand(text); ok {
		unlock := a.lockSession(msg.SessionID)
		defer unlock()
		return a.handleExamples(ctx, channelDelivery{
			SessionID: msg.SessionID,
			UserID:    msg.UserID,
			ChatID:    msg.ChatID,
			ThreadID:  msg.ThreadID,
		}, arg)
	}

	if cmd, ok := parseCommand(text); ok {
		switch cmd {
		case "/new", "/clear":
			unlock := a.lockSession(msg.SessionID)
			defer unlock()
			a.coalesceClear(msg.SessionID)
			distilled := false
			if a.selfNotes != nil {
				distilled = a.distillSelf(ctx, msg.SessionID)
			}
			if err := a.sessions.Reset(ctx, msg.SessionID); err != nil {
				return "", err
			}
			if distilled {
				return "session reset — personality distilled into SELF.md", nil
			}
			return "session reset", nil
		case "/status":
			unlock := a.lockSession(msg.SessionID)
			defer unlock()
			return a.status(ctx, msg.SessionID)
		case "/tools":
			unlock := a.lockSession(msg.SessionID)
			defer unlock()
			return a.listTools(), nil
		case "/perf":
			unlock := a.lockSession(msg.SessionID)
			defer unlock()
			return a.formatPerf(), nil
		case "/memstats":
			unlock := a.lockSession(msg.SessionID)
			defer unlock()
			return a.formatMemStats(ctx), nil
		case "/toolstats":
			unlock := a.lockSession(msg.SessionID)
			defer unlock()
			return a.formatToolStats(), nil
		case "/help":
			unlock := a.lockSession(msg.SessionID)
			defer unlock()
			return helpText, nil
		}
	}

	// Interrupt + coalesce + settle for multi-bubble asks (skip cron/reactions).
	if a.coalesceSettle > 0 && !skipCoalesce(text) {
		joined, run, err := a.coalesceAccept(ctx, msg)
		if err != nil {
			return "", err
		}
		if !run {
			return "", nil
		}
		msg = joined
		text = strings.TrimSpace(msg.Text)
	}

	return a.runTurn(ctx, msg, text)
}

// runTurn executes one model turn for an already-coalesced (or single) message.
func (a *Agent) runTurn(ctx context.Context, msg channel.Message, text string) (string, error) {
	storeText := messageStoreText(msg)

	unlock := a.lockSession(msg.SessionID)
	defer unlock()

	turnCtx, finish := a.beginTurn(ctx, msg.SessionID, storeText, msg.Images)
	defer func() {
		if finish() {
			a.log.Info("agent turn cancelled", "session_id", msg.SessionID)
		}
	}()

	history, err := a.sessions.Messages(turnCtx, msg.SessionID)
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
	// Everything appended from here is re-evaluated every turn, so its size —
	// not the total prompt — is what first_token_ms actually measures.
	shape := promptShape{stableEnd: len(messages)}
	hydrateQuery := text
	if hydrateQuery == "" {
		hydrateQuery = storeText
	}
	if a.memory != nil {
		entries, err := a.memory.Hydrate(turnCtx, hydrateQuery, 30)
		if err != nil {
			a.log.Warn("memory hydrate failed", "err", err)
		} else if block := memory.FormatHydration(entries); block != "" {
			shape.hydration = (len(block) + 3) / 4
			messages = append(messages, provider.Message{
				Role:    provider.RoleSystem,
				Content: block,
			})
		}
	}
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
	// Clock footer (not stored in history): after the user turn so the model
	// reads intent first. Leading with [current time] primed calendar/tool
	// fixation on small local models. Fresh each turn for "what time is it?".
	messages = append(messages, provider.Message{
		Role:    provider.RoleSystem,
		Content: temporalAnchor(time.Now().In(a.loc), a.tzName),
	})

	var toolDefs []provider.ToolDef
	if a.tools != nil && !channel.NoToolsFrom(ctx) {
		toolDefs = a.tools.Tools()
	}
	if turnSource(text) == "cron" && len(toolDefs) > 0 {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: cronToolFirstNote,
		})
	}
	shape.schemas = mcp.EstimateToolSchemaTokens(toolDefs)

	a.log.Debug("agent complete",
		"session_id", msg.SessionID,
		"history_messages", len(history),
		"tools", len(toolDefs),
		"est_tokens", estTokens(messages)+shape.schemas,
	)

	reply, err := a.runLoop(turnCtx, msg.SessionID, messages, toolDefs, shape, turnSource(text))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", nil
		}
		return "", err
	}

	if err := a.sessions.Append(turnCtx, msg.SessionID,
		session.Message{Role: session.RoleUser, Content: storeText},
		session.Message{Role: session.RoleAssistant, Content: reply},
	); err != nil {
		if errors.Is(err, context.Canceled) {
			return "", nil
		}
		return "", err
	}
	return reply, nil
}

// promptShape describes how much of the assembled prompt is cacheable. The
// prefix (persona + summary + history) is byte-stable across turns, so
// first_token_ms tracks the volatile remainder, not the total prompt size.
type promptShape struct {
	stableEnd int // index in messages where the cacheable prefix ends
	hydration int // est tokens in the memory hydration block
	schemas   int // est tokens in the tool schema block
}

func (a *Agent) runLoop(ctx context.Context, sessionID string, messages []provider.Message, toolDefs []provider.ToolDef, shape promptShape, source string) (string, error) {
	streamer, canStream := a.completer.(provider.Streamer)
	writer, hasWriter := channel.ReplyWriterFrom(ctx)
	progress, hasProgress := channel.ProgressWriterFrom(ctx)
	status, hasStatus := channel.StatusWriterFrom(ctx)
	nudged := false
	sawTools := false

	// Latency accounting: local models spend most of a turn in prefill/decode,
	// so split model vs tool time to know which one to attack.
	turnStart := time.Now()
	iters := 0
	var modelTime, toolTime time.Duration
	var firstTokenMS int64
	var volatileEst int
	cold := !a.warmed.Load()
	defer func() {
		totalMS := time.Since(turnStart).Milliseconds()
		a.log.Info("turn perf",
			"iterations", iters,
			"model_ms", modelTime.Milliseconds(),
			"tool_ms", toolTime.Milliseconds(),
			"total_ms", totalMS,
			"hydration_est_tokens", shape.hydration,
		)
		if a.perf != nil {
			a.perf.append(perfRecord{
				when:         turnStart,
				totalMS:      totalMS,
				modelMS:      modelTime.Milliseconds(),
				toolMS:       toolTime.Milliseconds(),
				iters:        iters,
				firstTokenMS: firstTokenMS,
				volatileEst:  volatileEst,
				source:       source,
				cold:         cold,
			})
		}
	}()

	// Names to force on the next call, set when a tool name failed to resolve.
	var forceNames []string
	budgetWarned := false
	// The loop grants maxToolIters tool rounds plus one landing call: tools are
	// withheld on that last call so the model must answer with text — the turn
	// ends with a real reply (and persisted history) instead of an error that
	// throws away every tool result it just gathered.
	for iter := 0; iter <= a.maxToolIters; iter++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		final := iter == a.maxToolIters
		iters = iter + 1
		bounded := collapseOldToolResults(messages)
		if final {
			forceNames = nil
			bounded = append(bounded, provider.Message{
				Role:    provider.RoleSystem,
				Content: fmt.Sprintf(budgetExhaustedNote, a.maxToolIters),
			})
		}
		req := provider.Request{Messages: bounded, Tools: toolDefs, ForceToolNames: forceNames}
		if final {
			req.Tools = nil
		}
		// One-shot: a repair either lands or the turn continues unconstrained.
		constrained := len(forceNames) > 0
		forceNames = nil
		promptTokens := estTokens(bounded) + shape.schemas
		// The re-evaluated remainder: hydration + clock + user message, plus
		// any tool results appended by earlier iterations.
		volatileTokens := estTokens(bounded[min(shape.stableEnd, len(bounded)):])

		var (
			res          *provider.Result
			err          error
			firstTokenAt time.Time
		)
		// Prefill is silent, so a slow turn looks frozen until the first token.
		stopNotice := func() {}
		if iter == 0 && hasStatus {
			stopNotice = a.startSpinupNotice(ctx, status)
		}
		callStart := time.Now()
		// Stream when enabled and a channel writer is present. Tool-call
		// responses still come back on the same stream path; onProgress is
		// skipped once tool deltas appear (see provider.CompleteStream).
		streamedRound := a.streamReplies && canStream && hasWriter && !constrained
		if streamedRound {
			tw, hasThinking := writer.(channel.ThinkingWriter)
			res, err = streamer.CompleteStream(ctx, req, func(content, thinking string) error {
				if firstTokenAt.IsZero() && (content != "" || thinking != "") {
					firstTokenAt = time.Now()
					stopNotice()
				}
				if hasThinking {
					return tw.UpdateThinking(ctx, thinking, content)
				}
				return writer.Update(ctx, content)
			})
		} else {
			res, err = a.completer.Complete(ctx, req)
		}
		callDur := time.Since(callStart)
		stopNotice()
		modelTime += callDur
		if err != nil {
			return "", err
		}
		a.warmed.Store(true)
		if iter == 0 {
			volatileEst = volatileTokens
			if !firstTokenAt.IsZero() {
				firstTokenMS = firstTokenAt.Sub(callStart).Milliseconds()
			}
		}
		perf := []any{
			"iteration", iter + 1,
			"dur_ms", callDur.Milliseconds(),
			"prompt_est_tokens", promptTokens,
			"schema_est_tokens", shape.schemas,
			"volatile_est_tokens", volatileTokens,
			"tool_schemas", len(toolDefs),
			"content_chars", len(res.Content),
			"thinking_chars", len(res.Thinking),
			"tool_calls", len(res.ToolCalls),
			"finish_reason", res.FinishReason,
		}
		if constrained {
			perf = append(perf, "forced_tool_names", len(req.ForceToolNames))
		}
		if !firstTokenAt.IsZero() {
			// Streaming only: prefill+queue time before the first delta.
			perf = append(perf, "first_token_ms", firstTokenAt.Sub(callStart).Milliseconds())
		}
		a.log.Info("model call", perf...)
		if res.FinishReason == "length" {
			a.log.Warn("model hit max_tokens (reply may be truncated)",
				"finish_reason", res.FinishReason,
				"chars", len(res.Content),
				"iteration", iter+1,
			)
		}
		// Models sometimes print the tool call instead of emitting one. Left
		// alone that JSON becomes the visible reply — the agent answering in
		// wire format — so run it as the call it plainly is. Not on the landing
		// call: nothing can execute there, so text (even ugly) must stand.
		if len(res.ToolCalls) == 0 && a.tools != nil && !final {
			if call, ok := salvageToolCall(res.Content, toolDefs, messages); ok {
				a.log.Warn("model printed a tool call instead of emitting one; executing it",
					"name", call.Name,
					"chars", len(res.Content),
					"iteration", iter+1,
				)
				res.ToolCalls = []provider.ToolCall{call}
				res.Content = ""
			}
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
			// or falsely claims one already ran ("I've created…") without any
			// tool_calls this turn — common small-model failure. Nudge once
			// before tools. After tools, only catch deferrals ("give me a
			// moment…") that leave the human hanging — giving up is fine;
			// stalling is not. (Do not use promisesToolCall after tools: a
			// honest "google__sheets_… failed" final answer contains "__".)
			// Cron live-data jobs are a separate miss: the model drafts the
			// digest (fake scores, agenda) with zero theater cues.
			preToolTheater := !sawTools && (promisesToolCall(res.Content, res.Thinking) || claimsToolSuccess(res.Content))
			cronSkippedLive := !sawTools && source == "cron" && len(toolDefs) > 0 &&
				cronJobImpliesLiveTools(lastUserContent(messages))
			deferral := sawTools && defersPendingWork(res.Content)
			if (preToolTheater || deferral || cronSkippedLive) && !nudged {
				a.log.Warn("model narrated tool action in prose without calling",
					"chars", len(res.Content),
					"iteration", iter+1,
					"saw_tools", sawTools,
					"deferral", deferral,
					"cron_skipped_live", cronSkippedLive,
				)
				nudged = true
				messages = append(messages, provider.Message{
					Role:    provider.RoleAssistant,
					Content: res.Content,
				})
				nudge := "[system] You described or claimed a tool action, but no tool call was made — nothing actually happened. " +
					"Emit the real tool call now using the exact name from the tools list. " +
					"Do not narrate and never report results you did not receive from a tool."
				if deferral {
					nudge = "[system] You said you would continue (try again / one moment / access next), but no tool call was made — the human is left hanging. " +
						"Act now: emit the real tool call using the exact name from the tools list, " +
						"OR give a final answer that reports the tool error and stops. Giving up is fine. " +
						"Do not ask for a moment or promise another attempt without calling a tool."
				} else if cronSkippedLive {
					nudge = "[system] This scheduled job needs live data, but you wrote the user-facing result without calling any tools. " +
						"Emit the real tool calls now using exact names from the tools list. " +
						"Do not invent metrics, events, or search results. After tools return, then write the report. " +
						"If a tool fails, report the failure."
				}
				messages = append(messages, provider.Message{
					Role:    provider.RoleSystem,
					Content: nudge,
				})
				continue
			}
			if deferral && nudged {
				// Second stall after nudge — don't ship "give me a moment" as the reply.
				a.log.Warn("model deferred again after nudge; forcing give-up",
					"chars", len(res.Content),
					"iteration", iter+1,
				)
				return "I couldn't finish that — tools failed and I stalled instead of retrying or giving up clearly. Please try again.", nil
			}
			return res.Content, nil
		}
		if a.tools == nil {
			return "", fmt.Errorf("agent: model requested tools but none are configured")
		}
		if final {
			// Tools were withheld from the landing call; a tool_call reply here
			// means the provider ignored that, so stop rather than loop on.
			return "", fmt.Errorf("agent: exceeded TOOL_MAX_ITERATIONS (%d)", a.maxToolIters)
		}

		messages = append(messages, provider.Message{
			Role:      provider.RoleAssistant,
			Content:   res.Content,
			ToolCalls: res.ToolCalls,
		})

		// Streamed rounds already showed the model's narration via the reply
		// writer; on the Complete path it would otherwise be invisible, so put
		// its first line in the trace — the "why" ahead of the ✓/✗ marks.
		if hasProgress && !streamedRound && a.toolTrace != ToolTraceOff {
			if reason := firstLine(res.Content); reason != "" {
				_ = progress.UpdateProgress(ctx, reason)
			}
		}

		compactHeader := false
		a.markToolsStarted(sessionID)
		for _, call := range res.ToolCalls {
			a.log.Info("tool call",
				"name", call.Name,
				"id", call.ID,
				"iteration", iter+1,
			)
			// Show forward motion during long tool chains — with thinking
			// disabled this is the only signal the user gets while waiting.
			// TOOL_TRACE=compact|off hides tool names (semi-quiet / fully quiet).
			if hasProgress {
				switch a.toolTrace {
				case ToolTraceFull:
					_ = progress.UpdateProgress(ctx, toolProgressStart(call.Name))
				case ToolTraceCompact:
					if !compactHeader {
						_ = progress.UpdateProgress(ctx, compactCallsHeader)
						compactHeader = true
					}
				}
			}
			args := json.RawMessage(call.Arguments)
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			toolStart := time.Now()
			out, err := a.tools.Call(ctx, call.Name, args)
			toolDur := time.Since(toolStart)
			toolTime += toolDur
			if err != nil {
				if errors.Is(err, context.Canceled) || ctx.Err() != nil {
					return "", context.Canceled
				}
				out = fmt.Sprintf("tool error: %v", err)
				a.log.Warn("tool call failed", "name", call.Name, "dur_ms", toolDur.Milliseconds(), "err", err)
				// Constrain only to a tight nearest shortlist (see mcp.suggest).
				// Never lock the next turn to an entire wrong server (e.g. every
				// strava__* tool after a calendar hallucination).
				var unknown *mcp.UnknownToolError
				if errors.As(err, &unknown) && len(unknown.Candidates) > 0 {
					forceNames = unknown.Candidates
					a.log.Info("constraining retry to nearest tool names",
						"requested", unknown.Name,
						"candidates", len(unknown.Candidates),
					)
				}
			} else {
				a.log.Info("tool done",
					"name", call.Name,
					"dur_ms", toolDur.Milliseconds(),
					"result_chars", len(out),
				)
			}
			if hasProgress {
				switch a.toolTrace {
				case ToolTraceFull:
					_ = progress.UpdateProgress(ctx, toolProgressDone(toolDur, len(out), err != nil))
				case ToolTraceCompact:
					mark := "✓"
					if err != nil {
						mark = "✗"
					}
					_ = progress.UpdateProgress(ctx, mark)
				}
			}
			sawTools = true
			messages = append(messages, provider.Message{
				Role:       provider.RoleTool,
				Content:    out,
				ToolCallID: call.ID,
			})
		}
		// Past ~70% of the budget, tell the model how many rounds remain so it
		// wraps up on its own instead of slamming into the landing call.
		if warnAt := a.maxToolIters * 7 / 10; !budgetWarned && iters >= warnAt && a.maxToolIters-iters >= 1 {
			budgetWarned = true
			messages = append(messages, provider.Message{
				Role: provider.RoleSystem,
				Content: fmt.Sprintf(
					"[system] Tool budget: %d of %d tool rounds used this turn; %d remain before you must answer. Finish the most important remaining step or wrap up now.",
					iters, a.maxToolIters, a.maxToolIters-iters),
			})
		}
	}
	return "", fmt.Errorf("agent: exceeded TOOL_MAX_ITERATIONS (%d)", a.maxToolIters)
}

func (a *Agent) status(ctx context.Context, sessionID string) (string, error) {
	n, histEst, err := a.sessions.Stats(ctx, sessionID)
	if err != nil {
		return "", err
	}
	var budget mcp.SchemaBudget
	if a.tools != nil {
		budget = mcp.EstimateSchemaBudget(a.tools.Tools())
	}
	uptime := formatUptime(time.Since(a.startedAt))
	var turns uint64
	if a.perf != nil {
		turns = a.perf.turnCount()
	}
	line := fmt.Sprintf(
		"uptime=%s model=%s history_messages=%d history_est_tokens=%d tools=%d schema_est_tokens=%d turns=%d",
		uptime, a.model, n, histEst, budget.Tools, budget.EstTokens, turns,
	)
	if mb, ok := selfRSSMB(); ok {
		line += fmt.Sprintf(" rss_mb=%d", mb)
	}
	return line, nil
}

func (a *Agent) listTools() string {
	if a.tools == nil {
		return "tools: (none)"
	}
	defs := a.tools.Tools()
	if len(defs) == 0 {
		return "tools: (none)"
	}
	budget := mcp.EstimateSchemaBudget(defs)
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "tools (%d) schema_est_tokens≈%d (chars/4)\n", budget.Tools, budget.EstTokens)
	if len(budget.ByServer) > 0 {
		b.WriteString("by server:\n")
		for _, s := range budget.ByServer {
			fmt.Fprintf(&b, "  %s: %d tools ≈ %d\n", s.Server, s.Tools, s.EstTokens)
		}
	}
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

// startSpinupNotice posts one status line so a silent prefill does not look
// frozen, and returns a stop func (safe to call more than once) that clears the
// line again so the reply replaces it.
//
// The first turn of the process is known-cold — model load and/or an empty
// prompt cache — so it posts at once. Later turns can be just as slow on a
// cache miss, but nothing in an OpenAI-compatible API reveals that (Ollama
// reports the model resident either way), so they post only after staying
// silent past spinupNotice. Lines are picked at random from spinupColdNotes /
// spinupSlowNotes so the wait text does not go stale.
func (a *Agent) startSpinupNotice(ctx context.Context, status channel.StatusWriter) func() {
	if a.spinupNotice <= 0 {
		return func() {}
	}
	// posted/stopped guard the timer goroutine against a concurrent stop, so a
	// notice can never land after the model has already spoken.
	var (
		mu      sync.Mutex
		posted  bool
		stopped bool
	)
	set := func(note string) {
		mu.Lock()
		if stopped {
			mu.Unlock()
			return
		}
		posted = true
		mu.Unlock()
		// UpdateStatus only caches text; the channel flushes it out of band, so
		// this never puts Telegram latency in front of the model call.
		if err := status.UpdateStatus(ctx, note); err != nil {
			a.log.Debug("spinup notice skipped", "err", err)
		}
	}
	takeDown := func() {
		mu.Lock()
		if stopped {
			mu.Unlock()
			return
		}
		stopped = true
		had := posted
		mu.Unlock()
		if !had {
			return
		}
		if err := status.UpdateStatus(ctx, ""); err != nil {
			a.log.Debug("spinup notice clear skipped", "err", err)
		}
	}
	if !a.warmed.Load() {
		set(pickSpinupNote(spinupColdNotes))
		return takeDown
	}
	done := make(chan struct{})
	go func() {
		t := time.NewTimer(a.spinupNotice)
		defer t.Stop()
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-t.C:
		}
		set(pickSpinupNote(spinupSlowNotes))
	}()
	wake := sync.OnceFunc(func() { close(done) })
	return func() {
		wake()
		takeDown()
	}
}

// salvageToolCall recovers a tool call the model wrote as text. The name must
// look like a tool — published, or at least carrying a server prefix — so an
// ordinary reply that happens to contain JSON is never hijacked into a call.
// An unpublished but prefixed name is still worth running: the host answers with
// the real names, which is how the model gets corrected.
//
// hints come from the latest assistant message (name printed last turn, args-only
// JSON this turn). System nudge text is ignored so examples like garmin__sleep_get
// do not steal the call.
func salvageToolCall(content string, defs []provider.ToolDef, messages []provider.Message) (provider.ToolCall, bool) {
	call, ok := provider.ParseToolCallTextHinted(content, lastAssistantToolHints(messages))
	if !ok {
		return provider.ToolCall{}, false
	}
	if strings.Contains(call.Name, "__") {
		return call, true
	}
	for _, def := range defs {
		if def.Name == call.Name {
			return call, true
		}
	}
	return provider.ToolCall{}, false
}

func lastAssistantToolHints(messages []provider.Message) []string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != provider.RoleAssistant {
			continue
		}
		names := provider.PrefixedToolNames(messages[i].Content)
		if len(names) == 1 {
			return names
		}
		return nil
	}
	return nil
}

// toolProgressStart is the trace line shown before a tool call runs.
func toolProgressStart(name string) string {
	return "→ " + name
}

// toolProgressDone summarises a finished tool call for the visible trace.
func toolProgressDone(d time.Duration, resultChars int, failed bool) string {
	if failed {
		return fmt.Sprintf("✗ failed · %s", shortDuration(d))
	}
	return fmt.Sprintf("✓ %s · %s", shortDuration(d), shortChars(resultChars))
}

func shortDuration(d time.Duration) string {
	if d < time.Second {
		return d.Truncate(time.Millisecond).String()
	}
	return d.Truncate(100 * time.Millisecond).String()
}

func shortChars(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk chars", float64(n)/1000)
	}
	return fmt.Sprintf("%d chars", n)
}

// firstLine returns the first non-empty line of s, clipped for the trace.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return clipChars(line, 100)
		}
	}
	return ""
}

// clipChars truncates s to at most n runes (with ellipsis when clipped).
func clipChars(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func lastUserContent(messages []provider.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == provider.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

// cronJobBody returns the scheduled prompt without the runner wrapper. The
// wrapper itself mentions tools/metrics and must not trip live-data detection
// on a plain reminder.
func cronJobBody(userText string) string {
	t := strings.TrimSpace(userText)
	if !strings.HasPrefix(t, "[cron]") {
		return t
	}
	if i := strings.Index(t, "\n\n"); i >= 0 {
		return strings.TrimSpace(t[i+2:])
	}
	return t
}

// cronJobImpliesLiveTools reports whether a scheduled prompt is asking for
// fetched data (fitness, calendar, mail, search, sheets) rather than a
// no-tool reminder. Conservative cues — "submit my timecard" must not match.
func cronJobImpliesLiveTools(userText string) bool {
	text := strings.ToLower(cronJobBody(userText))
	if strings.TrimSpace(text) == "" {
		return false
	}
	cues := []string{
		"fetch", "search", "pull ", "query",
		"garmin", "strava", "ghealth",
		"calendar", "gmail", "inbox",
		"sheets", "ledger",
		"sleep", "hrv", "readiness", "body battery",
		"web_search", "google_search",
		"audit", "brief", "summarize",
	}
	for _, cue := range cues {
		if strings.Contains(text, cue) {
			return true
		}
	}
	return false
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
		"i'll try", "i will try", "i'm going to try", "i am going to try",
		"i will call", "i will pull", "i will query", "going to call", "about to call",
		"going to access", "i'll access", "i am going to access", "i'm going to access",
		"query body battery", "call this function", "calling the", "call the tool",
	}
	for _, cue := range cues {
		if strings.Contains(text, cue) {
			return true
		}
	}
	return defersPendingWork(content) || defersPendingWork(thinking)
}

// defersPendingWork reports hanging / "one moment" deferral prose — the model
// ends the turn promising more work without emitting tool_calls. Used after
// tools have already run so "I'll try that ID now…" cannot be the final reply.
func defersPendingWork(content string) bool {
	text := strings.ToLower(content)
	if strings.TrimSpace(text) == "" {
		return false
	}
	cues := []string{
		"give me one moment", "give me a moment", "give me a second", "give me a minute",
		"one moment while", "just a moment", "one sec while", "hang tight",
		"hold on while i", "stand by while", "bear with me",
		"i'll try again", "i will try again", "let me try again", "trying again now",
		"i am going to try", "i'm going to try", "going to try to access",
		"i am going to access", "i'm going to access", "going to access that",
		"while i confirm", "while i check the connection", "confirm the connection",
	}
	for _, cue := range cues {
		if strings.Contains(text, cue) {
			return true
		}
	}
	return false
}

// claimsToolSuccess reports whether content claims a completed side-effecting
// action ("I've created…") — a fabricated success when no tools ran this turn.
// Only checked when sawTools is false, so honest post-tool summaries never hit
// it. Content only: thinking may legitimately plan in past tense.
func claimsToolSuccess(content string) bool {
	text := strings.ToLower(content)
	if strings.TrimSpace(text) == "" {
		return false
	}
	cues := []string{
		"i've created", "i have created", "i've added", "i have added",
		"i've updated", "i have updated", "i've deleted", "i have deleted",
		"i've removed", "i have removed", "i've sent", "i have sent",
		"i've scheduled", "i have scheduled", "i've set up", "i have set up",
		"is now created", "has been created", "has been added", "has been sent",
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
