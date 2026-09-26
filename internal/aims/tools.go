package aims

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/provider"
)

const (
	// ToolLog records one scored event.
	ToolLog = "aim_log"
	// ToolHistory lists ledger rows with ids.
	ToolHistory = "aim_history"
)

// ToolDefs returns the ledger tools.
func ToolDefs() []provider.ToolDef {
	return []provider.ToolDef{
		{
			Name: ToolLog,
			Description: "Record one event and your opinion of it toward each aim it touches. " +
				"aims is an object {area: score} with score -3..+3. The area must be a live aim/<area>. " +
				"ref (Garmin activity id or start time) replaces the previous row with that ref. " +
				"event rewrites that id (re-score). Same day without a ref appends. " +
				"Optional metric/value/unit rides along (weight 191.4 lb). note is what you did (nudged|asked|offered|praised|quiet). " +
				RubricText,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"what":   map[string]any{"type": "string", "description": "what happened, in the human's words"},
					"aims":   map[string]any{"type": "object", "description": "area to score, e.g. {\"training\":2,\"weight\":1}"},
					"day":    map[string]any{"type": "string", "description": "local YYYY-MM-DD; default today"},
					"ref":    map[string]any{"type": "string", "description": "source id so a tool event is not logged twice"},
					"note":   map[string]any{"type": "string", "description": "nudged, asked, offered, praised, or quiet"},
					"metric": map[string]any{"type": "string"},
					"value":  map[string]any{"type": "number"},
					"unit":   map[string]any{"type": "string"},
					"event":  map[string]any{"type": "integer", "description": "rewrite this ledger id"},
				},
				"required": []string{"what", "aims"},
			},
		},
		{
			Name: ToolHistory,
			Description: "List ledger rows with ids so you can review or re-score. " +
				"Default is the last 14 days. Empty area lists every aim.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"area":  map[string]any{"type": "string"},
					"from":  map[string]any{"type": "string", "description": "YYYY-MM-DD inclusive"},
					"to":    map[string]any{"type": "string", "description": "YYYY-MM-DD inclusive"},
					"limit": map[string]any{"type": "integer"},
				},
			},
		},
	}
}

// IsAimTool reports whether name is a ledger tool.
func IsAimTool(name string) bool {
	return name == ToolLog || name == ToolHistory
}

// Tools adapts Store into agent tool calls.
type Tools struct {
	Store *Store
}

// Call executes aim_log or aim_history.
func (t Tools) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if t.Store == nil {
		return "", fmt.Errorf("aims: store not configured")
	}
	var args map[string]any
	if len(arguments) > 0 && string(arguments) != "null" {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return "", fmt.Errorf("aims: bad arguments: %w", err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	switch name {
	case ToolLog:
		return t.log(ctx, args)
	case ToolHistory:
		return t.history(ctx, args)
	default:
		return "", fmt.Errorf("aims: unknown tool %q", name)
	}
}

func (t Tools) log(ctx context.Context, args map[string]any) (string, error) {
	ev := Event{
		What:   stringArg(args, "what"),
		Day:    stringArg(args, "day"),
		Ref:    stringArg(args, "ref"),
		Note:   stringArg(args, "note"),
		Metric: stringArg(args, "metric"),
		Unit:   stringArg(args, "unit"),
		Source: sourceDefault,
	}
	if id, ok, err := intArg(args, "event"); err != nil {
		return "", err
	} else if ok {
		ev.ID = id
	}
	if v, ok := floatArg(args, "value"); ok {
		ev.Value = &v
	}
	scores, err := parseScores(args["aims"])
	if err != nil {
		return "", err
	}
	got, err := t.Store.Log(ctx, ev, scores)
	if err != nil {
		return "", err
	}
	return FormatLine(got), nil
}

func (t Tools) history(ctx context.Context, args map[string]any) (string, error) {
	area := stringArg(args, "area")
	from := stringArg(args, "from")
	to := stringArg(args, "to")
	limit := 0
	if n, ok, err := intArg(args, "limit"); err != nil {
		return "", err
	} else if ok {
		limit = int(n)
	}
	if from == "" && to == "" {
		today := time.Now().In(t.Store.loc).Format(dayLayout)
		start, err := addDays(today, -13)
		if err != nil {
			return "", err
		}
		from, to = start, today
	}
	rows, err := t.Store.History(ctx, area, from, to, limit)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "no ledger rows", nil
	}
	var b strings.Builder
	for _, ev := range rows {
		b.WriteString(FormatLine(ev))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// FormatLine is one history row: #id day what — area ±n · area ±n.
func FormatLine(ev Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s %s", ev.ID, ev.Day, ev.What)
	if ev.Metric != "" && ev.Value != nil {
		fmt.Fprintf(&b, " (%s %g %s)", ev.Metric, *ev.Value, strings.TrimSpace(ev.Unit))
	}
	if len(ev.Scores) > 0 {
		b.WriteString(" — ")
		parts := make([]string, len(ev.Scores))
		for i, sc := range ev.Scores {
			parts[i] = sc.Area + " " + formatSigned(sc.Value)
			if sc.Note != "" {
				parts[i] += " " + sc.Note
			}
		}
		b.WriteString(strings.Join(parts, " · "))
	}
	return b.String()
}

func parseScores(raw any) (map[string]int, error) {
	obj, ok := raw.(map[string]any)
	if !ok || len(obj) == 0 {
		return nil, fmt.Errorf("aims: aims object is required")
	}
	out := make(map[string]int, len(obj))
	for area, v := range obj {
		n, err := asInt(v)
		if err != nil {
			return nil, fmt.Errorf("aims: score for %s: %w", area, err)
		}
		out[area] = int(n)
	}
	return out, nil
}

func stringArg(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return strings.TrimSpace(s)
}

func floatArg(args map[string]any, key string) (float64, bool) {
	switch v := args[key].(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func intArg(args map[string]any, key string) (int64, bool, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	n, err := asInt(v)
	if err != nil {
		return 0, false, fmt.Errorf("aims: bad %s: %w", key, err)
	}
	return n, true, nil
}

func asInt(v any) (int64, error) {
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
		return strconv.ParseInt(strings.TrimSpace(x), 10, 64)
	default:
		return 0, fmt.Errorf("not a number")
	}
}
