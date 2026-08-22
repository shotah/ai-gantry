package selfnote

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

// ToolNote is the builtin tool name exposed to the model (not MCP-prefixed).
const ToolNote = "self_note"

// ToolDefs returns the builtin self-note tool schema.
func ToolDefs() []provider.ToolDef {
	return []provider.ToolDef{{
		Name: ToolNote,
		Description: "APPEND one short new line to SELF.md (does not overwrite or distill the file). " +
			"Only for a vibe, joke, ritual, or north-star aim that is NOT already in the SELF.md bullets in your prompt — skip if it is already there or only a paraphrase. " +
			"A north-star is one sentence for months (how you show up). Progress logs and one-off to-dos are memory_store or cron, not this. " +
			"/new distill merges (does not flatten jokes). Not for facts about the human — use memory_store for those.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"note": map[string]any{"type": "string", "description": "one NEW short line (dozen words or fewer); skip if already covered in SELF.md"},
			},
			"required": []string{"note"},
		},
	}}
}

// IsSelfTool reports whether name is the builtin self-note tool.
func IsSelfTool(name string) bool { return name == ToolNote }

// Tools adapts a Store into agent tool calls.
type Tools struct {
	Store *Store
}

// Call executes the builtin self-note tool.
func (t Tools) Call(_ context.Context, name string, arguments json.RawMessage) (string, error) {
	if name != ToolNote {
		return "", fmt.Errorf("selfnote: unknown tool %q", name)
	}
	if t.Store == nil {
		return "", fmt.Errorf("selfnote: store not configured")
	}
	var args struct {
		Note string `json:"note"`
	}
	if len(arguments) > 0 && string(arguments) != "null" {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return "", fmt.Errorf("selfnote: bad arguments: %w", err)
		}
	}
	if err := t.Store.Append(args.Note); err != nil {
		return "", err
	}
	return "appended one line to SELF.md (append-only; /new distill merges later). Skip next time if already listed.", nil
}

// ToolRunner is the downstream tool set the composite wraps.
type ToolRunner interface {
	Tools() []provider.ToolDef
	Call(ctx context.Context, name string, arguments json.RawMessage) (string, error)
	ToolCount() int
}

// Composite merges the builtin self-note tool with an optional other runner.
type Composite struct {
	Self  Tools
	Other ToolRunner
}

// Tools returns the self-note def first, then other tools.
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

// Call routes self_note to the store; everything else to Other.
func (c Composite) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if IsSelfTool(name) {
		return c.Self.Call(ctx, name, arguments)
	}
	if c.Other == nil {
		return "", fmt.Errorf("selfnote: no tool runner for %q", name)
	}
	return c.Other.Call(ctx, name, arguments)
}

// CallStats forwards MCP call accounting when Other exposes it (mcp.Host).
func (c Composite) CallStats() mcp.CallStats {
	if s, ok := c.Other.(interface{ CallStats() mcp.CallStats }); ok {
		return s.CallStats()
	}
	return mcp.CallStats{}
}

// ServerHealth forwards last-call state when Other exposes it (mcp.Host).
func (c Composite) ServerHealth() []mcp.ServerStatus {
	return mcp.ServerHealthOf(c.Other)
}
