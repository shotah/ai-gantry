package mcpenable

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

// ToolRunner is the rest of the catalog (MCP + other builtins).
type ToolRunner interface {
	Tools() []provider.ToolDef
	Call(ctx context.Context, name string, arguments json.RawMessage) (string, error)
	ToolCount() int
}

// Composite prepends mcp_enable to Other.
type Composite struct {
	Enable Tools
	Other  ToolRunner
}

// Tools returns mcp_enable first, then other defs (unfiltered — agent publishes).
func (c Composite) Tools() []provider.ToolDef {
	defs := []provider.ToolDef{ToolDef()}
	if c.Other == nil {
		return defs
	}
	return append(defs, c.Other.Tools()...)
}

// ToolCount returns the unfiltered catalog size.
func (c Composite) ToolCount() int { return len(c.Tools()) }

// Call routes mcp_enable; everything else to Other.
func (c Composite) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if name == ToolName {
		return c.Enable.Call(ctx, arguments)
	}
	if c.Other == nil {
		return "", fmt.Errorf("mcpenable: no tool runner for %q", name)
	}
	return c.Other.Call(ctx, name, arguments)
}

// CallStats forwards MCP accounting.
func (c Composite) CallStats() mcp.CallStats {
	if s, ok := c.Other.(interface{ CallStats() mcp.CallStats }); ok {
		return s.CallStats()
	}
	return mcp.CallStats{}
}

// ServerHealth forwards last-call state.
func (c Composite) ServerHealth() []mcp.ServerStatus {
	return mcp.ServerHealthOf(c.Other)
}
