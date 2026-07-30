package agent_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

// unknownThenOK rejects the first call the way the MCP host rejects a name it
// cannot resolve, then behaves normally.
type unknownThenOK struct {
	defs    []provider.ToolDef
	calls   []string
	unknown *mcp.UnknownToolError
}

func (f *unknownThenOK) Tools() []provider.ToolDef { return f.defs }

func (f *unknownThenOK) ToolCount() int { return len(f.defs) }

func (f *unknownThenOK) Call(_ context.Context, name string, _ json.RawMessage) (string, error) {
	f.calls = append(f.calls, name)
	if len(f.calls) == 1 {
		return "", f.unknown
	}
	return "hrv 62ms", nil
}

// forcedNamesSeen runs one turn where the model first invents a tool name, and
// reports ForceToolNames as seen by each model call.
func forcedNamesSeen(t *testing.T, candidates []string) ([][]string, string) {
	t.Helper()
	var seen [][]string
	n := 0
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		seen = append(seen, req.ForceToolNames)
		n++
		switch n {
		case 1:
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "mcp__get_hrv_and_body_battery", Arguments: `{}`},
			}}, nil
		case 2:
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c2", Name: "garmin__hrv_get", Arguments: `{"date":"2026-07-27"}`},
			}}, nil
		default:
			return &provider.Result{Content: "HRV was 62ms."}, nil
		}
	}}
	a, err := agent.New(agent.Options{
		Completer: fc,
		Sessions:  newMemHistory(),
		Tools: &unknownThenOK{
			defs: []provider.ToolDef{
				{Name: "garmin__hrv_get"},
				{Name: "garmin__wellness_get_body_battery"},
			},
			unknown: &mcp.UnknownToolError{
				Name:       "mcp__get_hrv_and_body_battery",
				Hint:       "no such tool; closest real names are: …",
				Candidates: candidates,
			},
		},
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "hrv yesterday?"})
	if err != nil {
		t.Fatal(err)
	}
	return seen, reply
}

// An unresolvable tool name arms a grammar-constrained retry, so the model
// physically cannot misspell it twice. The constraint is one-shot: forcing every
// later call would make the model unable to answer in prose at all.
func TestAgent_UnknownToolConstrainsNextCallOnly(t *testing.T) {
	candidates := []string{"garmin__wellness_get_body_battery", "garmin__hrv_get"}
	seen, reply := forcedNamesSeen(t, candidates)

	if reply != "HRV was 62ms." {
		t.Fatalf("reply = %q", reply)
	}
	if len(seen) != 3 {
		t.Fatalf("model calls = %d, want 3", len(seen))
	}
	if seen[0] != nil {
		t.Fatalf("first call forced %v, want unconstrained", seen[0])
	}
	if !slices.Equal(seen[1], candidates) {
		t.Fatalf("retry forced %v, want %v", seen[1], candidates)
	}
	if seen[2] != nil {
		t.Fatalf("third call forced %v, want the constraint released", seen[2])
	}
}

// Nothing close enough to name means nothing to constrain to — forcing a call
// out of the whole catalog would just be a different guess.
func TestAgent_UnknownToolWithNoCandidatesStaysFree(t *testing.T) {
	seen, _ := forcedNamesSeen(t, nil)
	for i, forced := range seen {
		if forced != nil {
			t.Fatalf("call %d forced %v, want unconstrained", i+1, forced)
		}
	}
}
