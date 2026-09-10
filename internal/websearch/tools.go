package websearch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

const toolDescription = "Web search. Exact name: web_search. " +
	"Do not invent google_search, duckduckgo, or MCP-prefixed names — leftover google-search__web_search still works. " +
	"Returns titles, URLs, and snippets. It does not update calendars. " +
	"If they asked to put a found address on a calendar event, call the calendar tool next."

// ToolDefs returns the builtin web_search schema.
func ToolDefs() []provider.ToolDef {
	return []provider.ToolDef{{
		Name:        ToolName,
		Description: toolDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": `Search query. For places/businesses, include the name and city/region (e.g. "Edgeworks Bellevue address").`,
				},
			},
			"required": []string{"query"},
		},
	}}
}

// Tools adapts the in-process search client into agent tool calls.
type Tools struct {
	svc *searchService
}

// Call executes web_search (and leftover MCP / invented aliases).
func (t Tools) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if !IsSearchTool(name) {
		return "", fmt.Errorf("websearch: unknown tool %q", name)
	}
	if t.svc == nil {
		return "", fmt.Errorf("websearch: service is not configured")
	}
	var args struct {
		Query string `json:"query"`
	}
	if len(arguments) > 0 && string(arguments) != "null" {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return "", fmt.Errorf("websearch: bad arguments: %w", err)
		}
	}
	return t.svc.Search(ctx, args.Query)
}

// ToolRunner is the downstream tool set the composite wraps.
type ToolRunner interface {
	Tools() []provider.ToolDef
	Call(ctx context.Context, name string, arguments json.RawMessage) (string, error)
	ToolCount() int
}

// Composite merges builtin web_search with an optional other runner.
type Composite struct {
	Search Tools
	Other  ToolRunner
}

// Tools returns web_search first, then other tools.
func (c Composite) Tools() []provider.ToolDef {
	defs := ToolDefs()
	if c.Other == nil {
		return defs
	}
	return append(defs, c.Other.Tools()...)
}

// ToolCount returns the number of tools exposed to the model.
func (c Composite) ToolCount() int {
	return len(c.Tools())
}

// Call routes web_search (and aliases) to the builtin; everything else to Other.
func (c Composite) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if IsSearchTool(name) {
		return c.Search.Call(ctx, name, arguments)
	}
	if c.Other == nil {
		return "", fmt.Errorf("websearch: no tool runner for %q", name)
	}
	return c.Other.Call(ctx, name, arguments)
}

// CallStats forwards MCP call accounting when Other exposes it.
func (c Composite) CallStats() mcp.CallStats {
	if s, ok := c.Other.(interface{ CallStats() mcp.CallStats }); ok {
		return s.CallStats()
	}
	return mcp.CallStats{}
}

// ServerHealth forwards last-call state when Other exposes it.
func (c Composite) ServerHealth() []mcp.ServerStatus {
	return mcp.ServerHealthOf(c.Other)
}
