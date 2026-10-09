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
	// ForgetAim drops the ledger for an aim when memory_forget removes
	// insight aim/<area>. Nil skips the cascade. aim/bootstrap is not an aim.
	ForgetAim func(ctx context.Context, area string) error
}

// ToolDefs returns the three builtin memory tool schemas.
func ToolDefs() []provider.ToolDef {
	return []provider.ToolDef{
		{
			Name: ToolStore,
			Description: "Store one atomic memory (fact|preference|person|episode|insight). " +
				"Same kind+subject replaces the live row (old row kept, superseded). " +
				"Facts about the human (food, hours, people, events) go here — not self_note. " +
				"Months-scale plans: kind=insight, subject=aim/<area> (the sentence stays here; what happened goes to aim_log). Events: fact subject=event/<slug>. " +
				"Waiting on someone else: fact subject=waiting/<slug>. A note for you to follow up: fact subject=follow/<slug>. " +
				"A thing THEY have to do, even in passing (call, return, renew): fact subject=todo/<slug> this turn — not follow/. Store it and say you added it; never ask whether to add it. Doable now: say do it now, not good luck and not later. A future day named in the words: no cron_schedule for it and do not offer a reminder; that day's planner sets the cue. " +
				"[todo] on [harness] is that whole list with #ids, so never memory_recall for it: same subject rewrites; done is memory_forget by the #id on [todo], only when they say so. " +
				"Priority is a marker leading the words: !! urgent, ! high, none normal (\"!! file the extension\") — theirs, or your read of the stakes. [todo] sorts by it. " +
				"Hours: preference subject=pref/hours as sleep:/work:/quiet: HH:MM-HH:MM lines. " +
				"Never auto-save guesses. Jokes go in self_note.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":    map[string]any{"type": "string", "description": "fact|preference|person|episode|insight"},
					"subject": map[string]any{"type": "string", "description": "short topic key, e.g. chris, aim/training, skill/gmail"},
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
			Description: "Delete memory by id or by query match. Prefer id when known. A [todo] item is its #id on [harness]: forget that id, never a query (a query takes other rows with it), no memory_recall first.",
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
		return fmt.Sprintf("stored id=%d kind=%s subject=%q (same subject replaces the live row)", e.ID, e.Kind, e.Subject), nil

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
			var doomed []Entry
			if e, gerr := t.Backend.Get(ctx, id); gerr == nil {
				doomed = []Entry{e}
			}
			if err := t.Backend.Forget(ctx, id); err != nil {
				return "", err
			}
			t.cascadeAims(ctx, doomed)
			return fmt.Sprintf("forgot id=%d", id), nil
		}
		query, _ := args["query"].(string)
		doomed, _ := t.Backend.Recall(ctx, query, 100)
		n, err := t.Backend.ForgetQuery(ctx, query)
		if err != nil {
			return "", err
		}
		t.cascadeAims(ctx, doomed)
		return fmt.Sprintf("forgot %d row(s) matching %q", n, query), nil

	default:
		return "", fmt.Errorf("memory: unknown tool %q", name)
	}
}

func (t Tools) cascadeAims(ctx context.Context, entries []Entry) {
	if t.ForgetAim == nil {
		return
	}
	for _, e := range entries {
		if e.Kind != KindInsight || !strings.HasPrefix(e.Subject, SubjectAimPrefix) {
			continue
		}
		area := strings.TrimPrefix(e.Subject, SubjectAimPrefix)
		if area == "" || area == "bootstrap" {
			continue
		}
		if err := t.ForgetAim(ctx, area); err != nil {
			continue
		}
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
