package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/shotah/ai-gantry/internal/provider"
)

// Tool names exposed to the model (builtin, not MCP-prefixed).
const (
	ToolStore  = "memory_store"
	ToolRecall = "memory_recall"
	ToolForget = "memory_forget"
)

// Tools adapts a Memory backend into agent tool defs / calls.
type Tools struct {
	Backend Memory
}

// ToolDefs returns the three builtin memory tool schemas.
func ToolDefs() []provider.ToolDef {
	return []provider.ToolDef{
		{
			Name:        ToolStore,
			Description: "Store one atomic memory (fact|preference|person|episode|insight). Use deliberately; never auto-save guesses.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":    map[string]any{"type": "string", "description": "fact|preference|person|episode|insight"},
					"subject": map[string]any{"type": "string", "description": "short topic key, e.g. chris, climbing, mom"},
					"content": map[string]any{"type": "string", "description": "one atomic statement"},
				},
				"required": []string{"kind", "subject", "content"},
			},
		},
		{
			Name:        ToolRecall,
			Description: "Recall memories by free-text query (FTS5 + recency).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer", "description": "max rows (default 20)"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        ToolForget,
			Description: "Delete memory by id or by query match. Prefer id when known.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":    map[string]any{"type": "integer", "description": "memory row id"},
					"query": map[string]any{"type": "string", "description": "delete FTS matches"},
				},
			},
		},
	}
}

// IsMemoryTool reports whether name is a builtin memory tool.
func IsMemoryTool(name string) bool {
	switch name {
	case ToolStore, ToolRecall, ToolForget:
		return true
	default:
		return false
	}
}

// Call executes a builtin memory tool.
func (t Tools) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if t.Backend == nil {
		return "", fmt.Errorf("memory: backend not configured")
	}
	var args map[string]any
	if len(arguments) > 0 && string(arguments) != "null" {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return "", fmt.Errorf("memory: bad arguments: %w", err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	switch name {
	case ToolStore:
		kind, _ := args["kind"].(string)
		subject, _ := args["subject"].(string)
		content, _ := args["content"].(string)
		e, err := t.Backend.Store(ctx, kind, subject, content)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("stored id=%d kind=%s subject=%q", e.ID, e.Kind, e.Subject), nil

	case ToolRecall:
		query, _ := args["query"].(string)
		limit := defaultRecallLimit
		switch v := args["limit"].(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case json.Number:
			if n, err := v.Int64(); err == nil {
				limit = int(n)
			}
		}
		entries, err := t.Backend.Recall(ctx, query, limit)
		if err != nil {
			return "", err
		}
		if len(entries) == 0 {
			return "no memories matched", nil
		}
		var b strings.Builder
		for _, e := range entries {
			_, _ = fmt.Fprintf(&b, "id=%d (%s) %s: %s\n", e.ID, e.Kind, e.Subject, e.Content)
		}
		return strings.TrimRight(b.String(), "\n"), nil

	case ToolForget:
		if idVal, ok := args["id"]; ok && idVal != nil {
			id, err := asInt64(idVal)
			if err != nil {
				return "", err
			}
			if err := t.Backend.Forget(ctx, id); err != nil {
				return "", err
			}
			return fmt.Sprintf("forgot id=%d", id), nil
		}
		query, _ := args["query"].(string)
		n, err := t.Backend.ForgetQuery(ctx, query)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("forgot %d row(s) matching %q", n, query), nil

	default:
		return "", fmt.Errorf("memory: unknown tool %q", name)
	}
}

func asInt64(v any) (int64, error) {
	switch x := v.(type) {
	case float64:
		return int64(x), nil
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	case json.Number:
		return x.Int64()
	case string:
		return strconv.ParseInt(x, 10, 64)
	default:
		return 0, fmt.Errorf("memory: invalid id %v", v)
	}
}
