package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/provider"
)

// Tool names exposed to the model.
const (
	ToolAdd    = "watch_add"
	ToolList   = "watch_list"
	ToolCancel = "watch_cancel"
)

// Tools adapts Store into agent tool defs / calls.
type Tools struct {
	Store *Store
}

// ToolDefs returns the three builtin watch tool schemas.
func ToolDefs() []provider.ToolDef {
	return []provider.ToolDef{
		{
			Name: ToolAdd,
			Description: "Subscribe to an MCP fetch tool. The kernel polls it on an interval " +
				"and wakes this chat only when new item ids appear. Quiet polls never call the model. " +
				"The first poll seeds the cursor (no flood of old items). " +
				"tool must be a prefixed MCP name (e.g. feeds__items_list). " +
				"args is the JSON object passed to that tool each tick. " +
				"interval: 15m (default), 1h, or seconds (minimum 1m).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tool": map[string]any{
						"type":        "string",
						"description": "prefixed MCP tool, e.g. feeds__items_list or twitter__posts_list",
					},
					"args": map[string]any{
						"type":        "object",
						"description": "arguments passed to the tool each poll (e.g. {\"url\":\"https://…/rss.xml\"})",
					},
					"interval": map[string]any{
						"type":        "string",
						"description": "how often to poll: 15m, 1h, or integer seconds (default 15m, min 1m)",
					},
					"label": map[string]any{
						"type":        "string",
						"description": "short name for watch_list (e.g. NWS Santa Clara)",
					},
				},
				"required": []string{"tool"},
			},
		},
		{
			Name:        ToolList,
			Description: "List active watches for this chat.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        ToolCancel,
			Description: "Stop a watch by id from watch_list.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer"},
				},
				"required": []string{"id"},
			},
		},
	}
}

// IsWatchTool reports whether name is a builtin watch tool.
func IsWatchTool(name string) bool {
	switch name {
	case ToolAdd, ToolList, ToolCancel:
		return true
	default:
		return false
	}
}

// Call executes a builtin watch tool.
func (t Tools) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if t.Store == nil {
		return "", fmt.Errorf("watch: store not configured")
	}
	var args map[string]any
	if len(arguments) > 0 && string(arguments) != "null" {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return "", fmt.Errorf("watch: bad arguments: %w", err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	switch name {
	case ToolAdd:
		delivery, ok := cron.DeliveryFrom(ctx)
		if !ok || delivery.SessionID == "" {
			return "", fmt.Errorf("watch: missing delivery context (add from an interactive turn)")
		}
		tool, _ := args["tool"].(string)
		label, _ := args["label"].(string)
		intervalStr, _ := args["interval"].(string)
		interval, err := ParseInterval(intervalStr)
		if err != nil {
			return "", err
		}
		rawArgs := json.RawMessage(`{}`)
		if v, ok := args["args"]; ok && v != nil {
			b, err := json.Marshal(v)
			if err != nil {
				return "", fmt.Errorf("watch: args: %w", err)
			}
			rawArgs = b
		}
		w, err := t.Store.Add(ctx, tool, rawArgs, label, interval, delivery)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("watching id=%d tool=%s interval=%s next_poll=%s label=%q",
			w.ID, w.Tool, time.Duration(w.IntervalSeconds)*time.Second,
			w.NextRunAt.UTC().Format(time.RFC3339), w.Label), nil

	case ToolList:
		delivery, ok := cron.DeliveryFrom(ctx)
		sessionID := ""
		if ok {
			sessionID = delivery.SessionID
		}
		list, err := t.Store.ListSession(ctx, sessionID, false)
		if err != nil {
			return "", err
		}
		if len(list) == 0 {
			return "no active watches", nil
		}
		var b strings.Builder
		for _, w := range list {
			_, _ = fmt.Fprintf(&b, "id=%d tool=%s interval=%s next=%s label=%q\n",
				w.ID, w.Tool, time.Duration(w.IntervalSeconds)*time.Second,
				w.NextRunAt.UTC().Format(time.RFC3339), w.Label)
		}
		return strings.TrimRight(b.String(), "\n"), nil

	case ToolCancel:
		id, err := asInt64(args["id"])
		if err != nil {
			return "", err
		}
		delivery, ok := cron.DeliveryFrom(ctx)
		if ok && delivery.SessionID != "" {
			w, err := t.Store.Get(ctx, id)
			if err != nil {
				return "", err
			}
			if w.SessionID != delivery.SessionID {
				return "", fmt.Errorf("watch: %d not in this session", id)
			}
		}
		if err := t.Store.Cancel(ctx, id); err != nil {
			return "", err
		}
		return fmt.Sprintf("cancelled id=%d", id), nil

	default:
		return "", fmt.Errorf("watch: unknown tool %q", name)
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
		return 0, fmt.Errorf("watch: invalid id %v", v)
	}
}
