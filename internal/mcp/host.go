// Package mcp hosts MCP stdio servers: manifest, spawn, list tools, call, truncate, restart.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shotah/ai-gantry/internal/provider"
)

// Tool is one MCP tool exposed to the model under a prefixed name.
type Tool struct {
	Name         string // server__tool
	Server       string
	OriginalName string
	Description  string
	InputSchema  map[string]any
}

// Options configures the MCP host.
type Options struct {
	ManifestPath      string
	Logger            *slog.Logger
	ResultMaxChars    int
	Dial              DialFunc // optional; defaults to CommandTransport dial
	RestartMaxBackoff time.Duration
}

// DialFunc connects to one MCP server. Tests inject in-memory dialers.
type DialFunc func(ctx context.Context, spec ServerSpec, stderr io.Writer) (Conn, error)

// Conn is the subset of an MCP client session the host needs.
type Conn interface {
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (string, error)
	Close() error
}

// Host supervises MCP servers and routes tool calls.
type Host struct {
	log            *slog.Logger
	resultMaxChars int
	dial           DialFunc
	maxBackoff     time.Duration

	mu      sync.RWMutex
	servers map[string]*managedServer
	tools   map[string]*Tool // prefixed name → tool
	skipped []ServerStatus   // boot fail-soft; not in the published catalog

	stats callStatsState
}

type managedServer struct {
	spec   ServerSpec
	conn   Conn
	callMu sync.Mutex // one in-flight CallTool (and restart) per stdio child
}

// Start loads the manifest, connects every server, and lists tools.
// Per-server connect failures are logged and skipped (fail-soft): one missing
// API key or broken binary must not take down the whole agent. Manifest load
// errors still fail Start.
func Start(ctx context.Context, opts Options) (*Host, error) {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	dial := opts.Dial
	if dial == nil {
		dial = defaultDial
	}
	backoff := opts.RestartMaxBackoff
	if backoff <= 0 {
		backoff = 30 * time.Second
	}

	manifest, err := LoadManifest(opts.ManifestPath)
	if err != nil {
		return nil, err
	}

	h := &Host{
		log:            log,
		resultMaxChars: opts.ResultMaxChars,
		dial:           dial,
		maxBackoff:     backoff,
		servers:        make(map[string]*managedServer, len(manifest.Servers)),
		tools:          make(map[string]*Tool),
	}
	h.initCallStats()

	var failed int
	for _, spec := range manifest.Servers {
		if err := h.connectServer(ctx, spec); err != nil {
			failed++
			h.skipped = append(h.skipped, ServerStatus{
				Name:  spec.Name,
				State: ServerSkipped,
				Note:  clipHealthNote(err.Error()),
			})
			h.log.Error("mcp server boot skipped", "server", spec.Name, "err", err)
			continue
		}
	}
	h.log.Info("mcp host ready",
		"servers", len(h.servers),
		"tools", len(h.tools),
		"skipped", failed,
	)
	return h, nil
}

// Tools returns provider tool definitions for the current catalog.
// Tools returns the published catalog in a stable name order. Order matters
// for latency, not just tidiness: the schema block is usually the largest part
// of the prompt and sits in the system message, so a map's randomized order
// would rewrite the prompt prefix every turn and defeat the provider's prompt
// cache (forcing a full re-prefill instead of a cache hit).
func (h *Host) Tools() []provider.ToolDef {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]provider.ToolDef, 0, len(h.tools))
	for _, t := range h.tools {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, provider.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ToolCount returns the number of registered tools.
func (h *Host) ToolCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.tools)
}

// Call executes a prefixed tool name and truncates the result for the model.
func (h *Host) Call(ctx context.Context, toolName string, arguments json.RawMessage) (string, error) {
	text, err := h.call(ctx, toolName, arguments)
	if err != nil {
		return "", err
	}
	return Truncate(text, h.resultMaxChars), nil
}

// CallRaw is Call without TOOL_RESULT_MAX_CHARS. Machine consumers (the watch
// poller) need intact JSON; the truncation marker starts with a raw newline
// that json.Unmarshal rejects as "invalid character '\\n' in string literal".
func (h *Host) CallRaw(ctx context.Context, toolName string, arguments json.RawMessage) (string, error) {
	return h.call(ctx, toolName, arguments)
}

func (h *Host) call(ctx context.Context, toolName string, arguments json.RawMessage) (string, error) {
	tool, resolved, ok := h.resolve(toolName)
	if !ok {
		hint, candidates := h.suggest(toolName)
		h.recordUnknownTool(len(candidates) > 0)
		return "", &UnknownToolError{Name: toolName, Hint: hint, Candidates: candidates}
	}
	if resolved != toolName {
		h.recordPrefixAlias()
		h.log.Info("mcp tool name aliased", "requested", toolName, "resolved", resolved)
	}

	args := map[string]any{}
	if len(arguments) > 0 && string(arguments) != "null" {
		if err := json.Unmarshal(arguments, &args); err != nil {
			outErr := fmt.Errorf("mcp: invalid arguments for %q: %w", toolName, err)
			h.recordOutcome(tool.Server, resolved, 0, outErr)
			return "", outErr
		}
	}
	start := time.Now()
	if ms := h.managed(tool.Server); ms != nil {
		ms.callMu.Lock()
		defer ms.callMu.Unlock()
	}
	text, err := h.callOnce(ctx, tool, args)
	if err != nil {
		if !isRestartableMCPError(err) {
			h.recordOutcome(tool.Server, resolved, time.Since(start), err)
			return "", err
		}
		h.log.Warn("mcp tool call failed; attempting restart", "tool", resolved, "server", tool.Server, "err", err)
		if rerr := h.restartServer(ctx, tool.Server); rerr != nil {
			outErr := fmt.Errorf("mcp: call %q failed: %v (restart: %w)", resolved, err, rerr)
			h.recordOutcome(tool.Server, resolved, time.Since(start), outErr)
			return "", outErr
		}
		// Tool map may have changed; re-resolve (keep alias rules).
		tool, _, ok = h.resolve(toolName)
		if !ok {
			outErr := fmt.Errorf("mcp: tool %q missing after restart", toolName)
			h.recordOutcome(tool.Server, resolved, time.Since(start), outErr)
			return "", outErr
		}
		text, err = h.callOnce(ctx, tool, args)
		if err != nil {
			h.recordOutcome(tool.Server, resolved, time.Since(start), err)
			return "", err
		}
	}
	h.recordOutcome(tool.Server, resolved, time.Since(start), nil)
	return text, nil
}

// isRestartableMCPError reports transport-death failures worth a reconnect.
// Cancel/deadline and ordinary tool/arg errors must not tear down the server.
func isRestartableMCPError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"eof",
		"broken pipe",
		"connection reset",
		"not connected",
		"use of closed network connection",
		"use of closed",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// resolve looks up a tool by exact prefixed name, then by a common local-model
// typo: underscores in the server prefix where the catalog uses hyphens
// (e.g. google_search__google_search → google-search__web_search).
// Only the prefix is rewritten; tool suffixes keep underscores.
// Failing that, a real tool name carrying an invented or missing prefix.
func (h *Host) resolve(toolName string) (*Tool, string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if tool, ok := h.tools[toolName]; ok {
		return tool, toolName, true
	}
	if alt, ok := hyphenatePrefix(toolName); ok {
		if tool, ok := h.tools[alt]; ok {
			return tool, alt, true
		}
	}
	return h.resolveByBaseNameLocked(toolName)
}

// resolveByBaseNameLocked repairs a call whose tool name is real but whose
// server prefix is invented or absent (mcp__get_hrv, get_hrv → garmin__hrv_get).
// Each failed call costs a whole model round-trip, which on a local model is the
// most expensive thing in a turn, so repair beats a retry hint when there is
// only one possible answer.
//
// Two cases are deliberately left to the model instead:
//   - The requested prefix is a real server. There the model chose a server on
//     purpose, and suggest()'s catalog for *that* server beats silently crossing
//     over to a different one.
//   - Two servers publish the same tool name (garmin and strava both have
//     get_activity). Guessing would be a coin flip.
//
// Callers hold h.mu.
func (h *Host) resolveByBaseNameLocked(toolName string) (*Tool, string, bool) {
	prefix, base, hasPrefix := strings.Cut(toolName, "__")
	if !hasPrefix {
		base = toolName
	} else if h.hasServerPrefixLocked(prefix) {
		return nil, "", false
	}
	base = strings.ToLower(base)
	if base == "" {
		return nil, "", false
	}
	var (
		found    *Tool
		resolved string
		matches  int
	)
	for name, tool := range h.tools {
		_, candidate, ok := strings.Cut(name, "__")
		if !ok || strings.ToLower(candidate) != base {
			continue
		}
		matches++
		found, resolved = tool, name
	}
	if matches != 1 {
		return nil, "", false
	}
	return found, resolved, true
}

// hasServerPrefixLocked reports whether any published tool uses this prefix.
// Callers hold h.mu.
func (h *Host) hasServerPrefixLocked(prefix string) bool {
	for name := range h.tools {
		if p, _, ok := strings.Cut(name, "__"); ok && p == prefix {
			return true
		}
	}
	return false
}

// hyphenatePrefix rewrites server__tool so underscores in the server prefix
// become hyphens. Returns ok=false when unchanged or when no __ separator.
func hyphenatePrefix(toolName string) (string, bool) {
	prefix, rest, ok := strings.Cut(toolName, "__")
	if !ok {
		return "", false
	}
	altPrefix := strings.ReplaceAll(prefix, "_", "-")
	if altPrefix == prefix {
		return "", false
	}
	return altPrefix + "__" + rest, true
}

// UnknownToolError reports a tool name that could not be resolved. Candidates
// holds the real names the model most plausibly meant, so a caller can constrain
// the retry to those instead of hoping the hint is read correctly.
type UnknownToolError struct {
	Name       string   // the name the model asked for
	Hint       string   // model-facing explanation, already catalog-aware
	Candidates []string // real prefixed names, best guess first; may be empty
}

func (e *UnknownToolError) Error() string {
	return fmt.Sprintf("mcp: unknown tool %q — %s", e.Name, e.Hint)
}

// suggest builds a model-facing hint for an unknown tool name, plus Candidates
// for a constrained retry. Hint may list a whole server catalog; Candidates are
// only the nearest few names — never the full prefix dump. Dumping every
// strava__* tool as Candidates trapped calendar asks on the wrong server after
// one bad guess (the model could not call google__* on the retry).
func (h *Host) suggest(toolName string) (string, []string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	prefix, _, hasPrefix := strings.Cut(toolName, "__")
	near := h.nearestLocked(toolName)
	var names []string
	const maxHintTools = 12
	if hasPrefix {
		for name := range h.tools {
			if strings.HasPrefix(name, prefix+"__") {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			sort.Strings(names)
			shown, extra := clipToolNames(names, maxHintTools)
			hint := fmt.Sprintf("no such tool; valid %s tools include: %s%s — if this was the wrong integration, use another server prefix; otherwise retry with one of these exact names",
				prefix, strings.Join(shown, ", "), extra)
			return hint, near
		}
		// Prefix miss that is only underscore-vs-hyphen: still list that
		// server's tools so the model keeps the exact names (not just prefixes).
		if alt, ok := hyphenatePrefix(toolName); ok {
			altPrefix, _, _ := strings.Cut(alt, "__")
			for name := range h.tools {
				if strings.HasPrefix(name, altPrefix+"__") {
					names = append(names, name)
				}
			}
			if len(names) > 0 {
				sort.Strings(names)
				shown, extra := clipToolNames(names, maxHintTools)
				hint := fmt.Sprintf("no such tool or server prefix %q (did you mean %q?); valid %s tools include: %s%s — if this was the wrong integration, use another server prefix; otherwise retry with one of these exact names",
					prefix, altPrefix, altPrefix, strings.Join(shown, ", "), extra)
				return hint, near
			}
		}
	}
	seen := make(map[string]bool)
	for name := range h.tools {
		p, _, _ := strings.Cut(name, "__")
		if !seen[p] {
			seen[p] = true
			names = append(names, p)
		}
	}
	sort.Strings(names)
	prefixes := "available server prefixes are: " + strings.Join(names, ", ")
	// A bare prefix list is useless to a model that invented the whole name, so
	// lead with the real tools its fragments point at.
	if len(near) > 0 {
		return fmt.Sprintf("no such tool; closest real names are: %s — retry with one of these exact names; %s",
			strings.Join(near, ", "), prefixes), near
	}
	return "no such tool or server prefix; " + prefixes, nil
}

func clipToolNames(names []string, limit int) (shown []string, extra string) {
	if limit < 1 || len(names) <= limit {
		return names, ""
	}
	return names[:limit], fmt.Sprintf(" (+%d more)", len(names)-limit)
}

// maxToolSuggestions bounds the hint: a long list is just more to hallucinate from.
const maxToolSuggestions = 5

// genericToolTokens appear all over any catalog, so matching on them ranks
// everything equally and says nothing.
var genericToolTokens = map[string]bool{
	"get": true, "list": true, "set": true, "search": true, "create": true,
	"update": true, "delete": true, "add": true, "and": true, "or": true,
	"my": true, "the": true, "data": true, "info": true,
}

// nearestLocked ranks published tools by how many meaningful name tokens they
// share with the requested name. A local model that invents a tool usually
// stitches real fragments together — mcp__get_hrv_and_body_battery is
// garmin__hrv_get plus garmin__wellness_get_body_battery — so the fragments point
// straight at the tools it actually wanted. Callers hold h.mu.
func (h *Host) nearestLocked(toolName string) []string {
	base := toolName
	if _, rest, ok := strings.Cut(toolName, "__"); ok {
		base = rest
	}
	want := meaningfulTokens(base)
	if len(want) == 0 {
		return nil
	}
	type scored struct {
		name  string
		score int
	}
	var ranked []scored
	for name := range h.tools {
		_, nameBase, _ := strings.Cut(name, "__")
		score := 0
		for tok := range meaningfulTokens(nameBase) {
			if want[tok] {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{name: name, score: score})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].name < ranked[j].name
	})
	if len(ranked) > maxToolSuggestions {
		ranked = ranked[:maxToolSuggestions]
	}
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.name
	}
	return out
}

// meaningfulTokens splits a tool name into scoreable lowercase tokens.
func meaningfulTokens(name string) map[string]bool {
	out := make(map[string]bool)
	for _, tok := range strings.Split(strings.ToLower(name), "_") {
		if tok == "" || genericToolTokens[tok] {
			continue
		}
		out[tok] = true
	}
	return out
}

func (h *Host) managed(name string) *managedServer {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.servers[name]
}

func (h *Host) callOnce(ctx context.Context, tool *Tool, args map[string]any) (string, error) {
	h.mu.RLock()
	ms, ok := h.servers[tool.Server]
	h.mu.RUnlock()
	if !ok || ms.conn == nil {
		return "", fmt.Errorf("mcp: server %q not connected", tool.Server)
	}
	return ms.conn.CallTool(ctx, tool.OriginalName, args)
}

// Close shuts down all MCP sessions.
func (h *Host) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var first error
	for name, ms := range h.servers {
		if ms.conn != nil {
			if err := ms.conn.Close(); err != nil && first == nil {
				first = fmt.Errorf("mcp: close %q: %w", name, err)
			}
			ms.conn = nil
		}
	}
	h.servers = nil
	h.tools = nil
	return first
}

func (h *Host) connectServer(ctx context.Context, spec ServerSpec) error {
	stderr := newLineLogger(h.log, spec.Name)
	conn, err := h.dial(ctx, spec, stderr)
	if err != nil {
		return err
	}
	listed, err := conn.ListTools(ctx)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("list tools: %w", err)
	}
	tools, before := filterTools(spec, listed)
	prefix := prefixFor(spec)

	h.mu.Lock()
	defer h.mu.Unlock()
	// Drop previous tools for this server (restart path).
	for name, t := range h.tools {
		if t.Server == spec.Name {
			delete(h.tools, name)
		}
	}
	for i := range tools {
		t := tools[i]
		t.Server = spec.Name
		prefixed, err := PrefixedName(prefix, t.OriginalName)
		if err != nil {
			_ = conn.Close()
			return err
		}
		t.Name = prefixed
		if _, exists := h.tools[prefixed]; exists {
			_ = conn.Close()
			return fmt.Errorf("tool name collision on %q", prefixed)
		}
		h.tools[prefixed] = &t
	}
	if existing, ok := h.servers[spec.Name]; ok {
		existing.spec = spec
		existing.conn = conn
	} else {
		h.servers[spec.Name] = &managedServer{spec: spec, conn: conn}
	}
	h.log.Info("mcp server connected",
		"server", spec.Name,
		"tools_listed", before,
		"tools_published", len(tools),
		"prefix", prefix,
	)
	return nil
}

func (h *Host) restartServer(ctx context.Context, name string) error {
	h.mu.Lock()
	ms, ok := h.servers[name]
	if ok && ms.conn != nil {
		_ = ms.conn.Close()
		ms.conn = nil
	}
	var spec ServerSpec
	if ok {
		spec = ms.spec
	}
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown server %q", name)
	}

	var last error
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= 4; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := h.connectServer(ctx, spec); err != nil {
			last = err
			h.log.Warn("mcp restart failed", "server", name, "attempt", attempt, "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > h.maxBackoff {
				backoff = h.maxBackoff
			}
			continue
		}
		return nil
	}
	return last
}

func defaultDial(ctx context.Context, spec ServerSpec, stderr io.Writer) (Conn, error) {
	// Do not bind the child to the boot/signal context: SIGTERM must let the
	// agent finish the in-flight turn before Host.Close kills MCP children.
	cmd := exec.Command(spec.Command, spec.Args...) //nolint:gosec // G204: command comes from operator mcp.toml
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Stderr = stderr

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "gantry", Version: "dev"}, nil)
	transport := &mcpsdk.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	return &sdkConn{session: session}, nil
}

type sdkConn struct {
	session *mcpsdk.ClientSession
}

func (c *sdkConn) ListTools(ctx context.Context) ([]Tool, error) {
	var out []Tool
	for tool, err := range c.session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		schema, err := schemaToMap(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q schema: %w", tool.Name, err)
		}
		out = append(out, Tool{
			OriginalName: tool.Name,
			Description:  tool.Description,
			InputSchema:  schema,
		})
	}
	return out, nil
}

func (c *sdkConn) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	res, err := c.session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return "", err
	}
	text := contentToString(res)
	if res.IsError {
		return "", fmt.Errorf("tool error: %s", text)
	}
	return text, nil
}

func (c *sdkConn) Close() error {
	return c.session.Close()
}

func schemaToMap(schema any) (map[string]any, error) {
	if schema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}, nil
	}
	if m, ok := schema.(map[string]any); ok {
		return m, nil
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func contentToString(res *mcpsdk.CallToolResult) string {
	if res == nil {
		return ""
	}
	var parts []string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcpsdk.TextContent:
			parts = append(parts, v.Text)
		default:
			b, err := json.Marshal(v)
			if err != nil {
				parts = append(parts, fmt.Sprintf("%v", v))
			} else {
				parts = append(parts, string(b))
			}
		}
	}
	if len(parts) == 0 && res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err == nil {
			return string(b)
		}
	}
	return strings.Join(parts, "\n")
}

type lineLogger struct {
	log    *slog.Logger
	server string
}

func newLineLogger(log *slog.Logger, server string) *lineLogger {
	return &lineLogger{log: log, server: server}
}

func (l *lineLogger) Write(p []byte) (int, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(p)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			l.log.Info("mcp stderr", "server", l.server, "line", line)
		}
	}
	return len(p), nil
}
