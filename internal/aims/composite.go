package aims

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

// ToolRunner is the rest of the catalog under the ledger tools.
type ToolRunner interface {
	Tools() []provider.ToolDef
	Call(ctx context.Context, name string, arguments json.RawMessage) (string, error)
	ToolCount() int
}

// Composite merges ledger tools with an optional other runner.
type Composite struct {
	Aims  Tools
	Other ToolRunner
}

// Tools returns aim defs first, then other tools.
func (c Composite) Tools() []provider.ToolDef {
	defs := ToolDefs()
	if c.Other == nil {
		return defs
	}
	return append(defs, c.Other.Tools()...)
}

// ToolCount returns the number of tools exposed to the model.
func (c Composite) ToolCount() int { return len(c.Tools()) }

// Call routes aim_* to the ledger; everything else to Other.
func (c Composite) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if IsAimTool(name) {
		return c.Aims.Call(ctx, name, arguments)
	}
	if c.Other == nil {
		return "", fmt.Errorf("aims: no tool runner for %q", name)
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
