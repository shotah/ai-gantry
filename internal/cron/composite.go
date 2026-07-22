package cron

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shotah/ai-gantry/internal/provider"
)

// ToolRunner is MCP (or other) tools merged with builtin cron tools.
type ToolRunner interface {
	Tools() []provider.ToolDef
	Call(ctx context.Context, name string, arguments json.RawMessage) (string, error)
	ToolCount() int
}

// Composite merges builtin cron tools with an optional other runner.
type Composite struct {
	Cron  Tools
	Other ToolRunner
}

// Tools returns cron defs first, then other tools.
func (c Composite) Tools() []provider.ToolDef {
	defs := ToolDefs()
	if c.Other == nil {
		return defs
	}
	return append(defs, c.Other.Tools()...)
}

// ToolCount returns the number of tools exposed to the model.
func (c Composite) ToolCount() int { return len(c.Tools()) }

// Call routes cron_* to the cron store; everything else to Other.
func (c Composite) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if IsCronTool(name) {
		return c.Cron.Call(ctx, name, arguments)
	}
	if c.Other == nil {
		return "", fmt.Errorf("cron: no tool runner for %q", name)
	}
	return c.Other.Call(ctx, name, arguments)
}
