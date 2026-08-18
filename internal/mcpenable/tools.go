package mcpenable

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/provider"
)

type sessionKey struct{}

// WithSession binds sessionID for mcp_enable calls.
func WithSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionKey{}, strings.TrimSpace(sessionID))
}

func sessionFrom(ctx context.Context) string {
	s, _ := ctx.Value(sessionKey{}).(string)
	return s
}

// SessionID returns the bound session, if any.
func SessionID(ctx context.Context) string { return sessionFrom(ctx) }

// Tools is the mcp_enable builtin.
type Tools struct {
	Store *Store
	Index func() []string
}

// ToolDef is the mcp_enable schema.
func ToolDef() provider.ToolDef {
	return provider.ToolDef{
		Name: ToolName,
		Description: "Enable MCP tool-name prefixes for this chat so their schemas are sent on the next model call. " +
			"Default hold is short (27h idle). hold=brief is 6h (this morning/afternoon only). hold=long is 76h (weekend). " +
			"Use brief when the job is only today-for-a-few-hours (flights this afternoon). " +
			"Pass every prefix this turn needs in one call. Do not enable a fat server (google) when google__calendar exists.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prefixes": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "index keys, e.g. google__calendar, garmin__sleep, flights",
				},
				"hold": map[string]any{
					"type":        "string",
					"description": "short (default, 27h), brief (6h), or long (76h)",
				},
			},
			"required": []string{"prefixes"},
		},
	}
}

// Call executes mcp_enable.
func (t Tools) Call(ctx context.Context, arguments json.RawMessage) (string, error) {
	if t.Store == nil {
		return "", fmt.Errorf("mcpenable: store not configured")
	}
	sessionID := sessionFrom(ctx)
	if sessionID == "" {
		return "", fmt.Errorf("mcpenable: no session")
	}
	var args struct {
		Prefixes []string `json:"prefixes"`
		Hold     string   `json:"hold"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return "", fmt.Errorf("mcpenable: bad arguments: %w", err)
	}
	index := []string(nil)
	if t.Index != nil {
		index = t.Index()
	}
	landed, failed, err := t.Store.Enable(ctx, sessionID, args.Prefixes, args.Hold, SourceAgent, time.Now(), index)
	if err != nil && len(landed) == 0 {
		return "", err
	}
	var b strings.Builder
	if len(landed) > 0 {
		fmt.Fprintf(&b, "enabled %s (%s) — schemas on the next model call", strings.Join(landed, ", "), normalizeHold(args.Hold))
	}
	if len(failed) > 0 {
		if b.Len() > 0 {
			b.WriteString(". ")
		}
		fmt.Fprintf(&b, "skipped: %s", strings.Join(failed, "; "))
	}
	if b.Len() == 0 {
		return "nothing enabled", nil
	}
	return b.String(), nil
}
