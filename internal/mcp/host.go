// Package mcp hosts MCP stdio servers: manifest, spawn, list tools, call, truncate, restart.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
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
}

type managedServer struct {
	spec ServerSpec
	conn Conn
}

// Start loads the manifest, connects every server (fail-fast), and lists tools.
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

	for _, spec := range manifest.Servers {
		if err := h.connectServer(ctx, spec); err != nil {
			_ = h.Close()
			return nil, fmt.Errorf("mcp: boot server %q: %w", spec.Name, err)
		}
	}
	h.log.Info("mcp host ready", "servers", len(h.servers), "tools", len(h.tools))
	return h, nil
}

// Tools returns provider tool definitions for the current catalog.
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
	return out
}

// ToolCount returns the number of registered tools.
func (h *Host) ToolCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.tools)
}

// Call executes a prefixed tool name and truncates the result.
func (h *Host) Call(ctx context.Context, toolName string, arguments json.RawMessage) (string, error) {
	h.mu.RLock()
	tool, ok := h.tools[toolName]
	h.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("mcp: unknown tool %q", toolName)
	}

	args := map[string]any{}
	if len(arguments) > 0 && string(arguments) != "null" {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return "", fmt.Errorf("mcp: invalid arguments for %q: %w", toolName, err)
		}
	}

	text, err := h.callOnce(ctx, tool, args)
	if err != nil {
		h.log.Warn("mcp tool call failed; attempting restart", "tool", toolName, "server", tool.Server, "err", err)
		if rerr := h.restartServer(ctx, tool.Server); rerr != nil {
			return "", fmt.Errorf("mcp: call %q failed: %v (restart: %w)", toolName, err, rerr)
		}
		// Tool map may have changed; re-resolve.
		h.mu.RLock()
		tool, ok = h.tools[toolName]
		h.mu.RUnlock()
		if !ok {
			return "", fmt.Errorf("mcp: tool %q missing after restart", toolName)
		}
		text, err = h.callOnce(ctx, tool, args)
		if err != nil {
			return "", err
		}
	}
	return Truncate(text, h.resultMaxChars), nil
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
	tools, err := conn.ListTools(ctx)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("list tools: %w", err)
	}

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
		prefixed, err := PrefixedName(spec.Name, t.OriginalName)
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
	h.servers[spec.Name] = &managedServer{spec: spec, conn: conn}
	h.log.Info("mcp server connected", "server", spec.Name, "tools", len(tools))
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
