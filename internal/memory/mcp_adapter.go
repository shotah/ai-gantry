package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/shotah/ai-gantry/internal/mcp"
)

// MCPCaller is the subset of mcp.Host needed by the memory adapter.
type MCPCaller interface {
	Call(ctx context.Context, toolName string, arguments json.RawMessage) (string, error)
}

// MCPAdapter routes memory tools to an MCP server from the manifest
// (MEMORY_BACKEND=mcp:<server-name>). Expected remote tools:
//
//	{server}__memory_store | {server}__memory_recall | {server}__memory_forget
type MCPAdapter struct {
	Caller MCPCaller
	Server string
}

// NewMCPAdapter builds an adapter for server name (without mcp: prefix).
func NewMCPAdapter(caller MCPCaller, server string) (*MCPAdapter, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return nil, fmt.Errorf("memory: mcp adapter requires server name")
	}
	if caller == nil {
		return nil, fmt.Errorf("memory: mcp adapter requires caller")
	}
	return &MCPAdapter{Caller: caller, Server: server}, nil
}

func (a *MCPAdapter) tool(name string) (string, error) {
	return mcp.PrefixedName(a.Server, name)
}

// Store calls remote memory_store.
func (a *MCPAdapter) Store(ctx context.Context, kind, subject, content string) (Entry, error) {
	name, err := a.tool(ToolStore)
	if err != nil {
		return Entry{}, err
	}
	args, _ := json.Marshal(map[string]any{
		"kind": kind, "subject": subject, "content": content,
	})
	out, err := a.Caller.Call(ctx, name, args)
	if err != nil {
		return Entry{}, err
	}
	e := Entry{Kind: kind, Subject: subject, Content: content, Source: SourceChat}
	if id, ok := parseIDFromText(out); ok {
		e.ID = id
	}
	return e, nil
}

// Recall calls remote memory_recall.
func (a *MCPAdapter) Recall(ctx context.Context, query string, limit int) ([]Entry, error) {
	name, err := a.tool(ToolRecall)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultRecallLimit
	}
	args, _ := json.Marshal(map[string]any{"query": query, "limit": limit})
	out, err := a.Caller.Call(ctx, name, args)
	if err != nil {
		return nil, err
	}
	return parseRecallText(out), nil
}

// Forget calls remote memory_forget by id.
func (a *MCPAdapter) Forget(ctx context.Context, id int64) error {
	name, err := a.tool(ToolForget)
	if err != nil {
		return err
	}
	args, _ := json.Marshal(map[string]any{"id": id})
	_, err = a.Caller.Call(ctx, name, args)
	return err
}

// ForgetQuery calls remote memory_forget by query.
func (a *MCPAdapter) ForgetQuery(ctx context.Context, query string) (int, error) {
	name, err := a.tool(ToolForget)
	if err != nil {
		return 0, err
	}
	args, _ := json.Marshal(map[string]any{"query": query})
	out, err := a.Caller.Call(ctx, name, args)
	if err != nil {
		return 0, err
	}
	return parseForgotCount(out), nil
}

// Hydrate uses recall with the current query (remote backends vary).
func (a *MCPAdapter) Hydrate(ctx context.Context, query string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = defaultHydrateLimit
	}
	q := strings.TrimSpace(query)
	if q == "" {
		q = "*"
	}
	return a.Recall(ctx, q, limit)
}

// Close is a no-op (MCP host owns the connection).
func (a *MCPAdapter) Close() error { return nil }

func parseIDFromText(s string) (int64, bool) {
	const prefix = "id="
	i := strings.Index(s, prefix)
	if i < 0 {
		return 0, false
	}
	rest := s[i+len(prefix):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(rest[:end], 10, 64)
	return n, err == nil
}

func parseForgotCount(s string) int {
	fields := strings.Fields(s)
	for i, f := range fields {
		if f == "forgot" && i+1 < len(fields) {
			n, err := strconv.Atoi(fields[i+1])
			if err == nil {
				return n
			}
		}
	}
	return 0
}

// parseRecallText turns line-oriented recall output into entries (best-effort).
func parseRecallText(s string) []Entry {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "no memories matched") {
		return nil
	}
	var out []Entry
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		e := Entry{Content: line, Source: SourceChat}
		if id, ok := parseIDFromText(line); ok {
			e.ID = id
		}
		out = append(out, e)
	}
	return out
}
