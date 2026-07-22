package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

// ToolRunner is MCP (or other) tools merged with builtin memory tools.
type ToolRunner interface {
	Tools() []provider.ToolDef
	Call(ctx context.Context, name string, arguments json.RawMessage) (string, error)
	ToolCount() int
}

// Composite merges builtin memory tools with an optional MCP/other runner.
// When HideMCPServer is set (mcp memory backend), that server's memory_* tools
// are omitted from the listing so the model only sees the unprefixed names.
type Composite struct {
	Memory        Tools
	Other         ToolRunner
	HideMCPServer string // optional: strip {server}__memory_* from Other.Tools()
}

// Tools returns memory defs first, then other tools.
func (c Composite) Tools() []provider.ToolDef {
	defs := ToolDefs()
	if c.Other == nil {
		return defs
	}
	for _, d := range c.Other.Tools() {
		if c.HideMCPServer != "" && isHiddenMemoryTool(c.HideMCPServer, d.Name) {
			continue
		}
		defs = append(defs, d)
	}
	return defs
}

// ToolCount returns the number of tools exposed to the model.
func (c Composite) ToolCount() int {
	return len(c.Tools())
}

// Call routes memory_* to the memory backend; everything else to Other.
func (c Composite) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if IsMemoryTool(name) {
		return c.Memory.Call(ctx, name, arguments)
	}
	if c.Other == nil {
		return "", fmt.Errorf("memory: no tool runner for %q", name)
	}
	return c.Other.Call(ctx, name, arguments)
}

func isHiddenMemoryTool(server, name string) bool {
	for _, t := range []string{ToolStore, ToolRecall, ToolForget} {
		pref, err := mcp.PrefixedName(server, t)
		if err == nil && name == pref {
			return true
		}
	}
	// Also hide if someone registered unprefixed names on that server listing.
	return strings.HasPrefix(name, server+"__memory_")
}
