package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

func TestAgent_FatToolResultsStayBounded(t *testing.T) {
	const maxToolChars = 200
	fat := strings.Repeat("Z", 50_000)

	var maxEst int
	n := 0
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		n++
		est := 0
		for _, m := range req.Messages {
			est += (len(m.Content) + 3) / 4
			if m.Role == provider.RoleTool && strings.Contains(m.Content, "Z") {
				if len(m.Content) > maxToolChars+50 {
					t.Fatalf("tool payload not truncated: %d chars", len(m.Content))
				}
			}
			if m.Role == provider.RoleTool && strings.Contains(m.Content, "[tool ") {
				// collapsed marker is small
				if len(m.Content) > 200 {
					t.Fatalf("collapsed tool too large: %q", m.Content)
				}
			}
		}
		if est > maxEst {
			maxEst = est
		}
		if n <= 6 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: fmt.Sprintf("c%d", n), Name: "dump__fat", Arguments: `{}`},
			}}, nil
		}
		return &provider.Result{Content: "done"}, nil
	}}

	tools := &truncTools{max: maxToolChars, fat: fat}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        tools,
		Model:        "m",
		MaxToolIters: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "dump"}); err != nil {
		t.Fatal(err)
	}
	// Fat dumps (50k) * 6 would be huge; with truncate + collapse, stay well under that.
	const budget = 20_000 // est tokens
	if maxEst > budget {
		t.Fatalf("prompt est_tokens=%d exceeded budget %d", maxEst, budget)
	}
}

type truncTools struct {
	max int
	fat string
}

func (t *truncTools) Tools() []provider.ToolDef {
	return []provider.ToolDef{{Name: "dump__fat", Parameters: map[string]any{"type": "object"}}}
}

func (t *truncTools) ToolCount() int { return 1 }

func (t *truncTools) Call(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	// Mirror mcp.Truncate behavior used in production.
	s := t.fat
	if t.max > 0 && len([]rune(s)) > t.max {
		const marker = "\n…[truncated]"
		keep := t.max - len([]rune(marker))
		if keep < 1 {
			keep = 1
		}
		r := []rune(s)
		s = string(r[:keep]) + marker
	}
	return s, nil
}
