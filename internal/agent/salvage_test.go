package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

// Observed on Qwen after a tool-call nudge: it printed the call as JSON with a
// "parameters" key and no tool_calls, and gantry shipped that JSON to Telegram as
// SAM's reply. The call must be executed instead — and since the printed name was
// not real either, the failure has to chain into the constrained retry.
func TestAgent_PrintedToolCallIsExecutedNotSpoken(t *testing.T) {
	printed := "{\n  \"name\": \"garmin__get_daily_activity\",\n  \"parameters\": {\n    \"date\": \"2026-07-28\"\n  }\n}"

	var forced [][]string
	n := 0
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		forced = append(forced, req.ForceToolNames)
		n++
		switch n {
		case 1:
			return &provider.Result{Content: printed, FinishReason: "stop"}, nil
		case 2:
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c2", Name: "garmin__activities_list", Arguments: `{"date":"2026-07-28"}`},
			}}, nil
		default:
			return &provider.Result{Content: "You rode 21mi yesterday."}, nil
		}
	}}
	tools := &unknownThenOK{
		defs: []provider.ToolDef{{Name: "garmin__activities_list"}},
		unknown: &mcp.UnknownToolError{
			Name:       "garmin__get_daily_activity",
			Hint:       "no such tool; valid garmin tools are: garmin__activities_list",
			Candidates: []string{"garmin__activities_list"},
		},
	}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        tools,
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "what did I ride?"})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(reply, "{") || strings.Contains(reply, "garmin__") {
		t.Fatalf("reply = %q, want prose — raw wire format must never reach the user", reply)
	}
	if reply != "You rode 21mi yesterday." {
		t.Fatalf("reply = %q", reply)
	}
	// The printed name was executed as written, which is what surfaces the real
	// catalog and arms the constrained retry.
	if len(tools.calls) != 2 || tools.calls[0] != "garmin__get_daily_activity" {
		t.Fatalf("tool calls = %v, want the printed name attempted first", tools.calls)
	}
	if len(forced) != 3 || len(forced[1]) != 1 || forced[1][0] != "garmin__activities_list" {
		t.Fatalf("forced names per call = %v, want the retry constrained", forced)
	}
}

// A reply that merely contains JSON is not a tool call. Hijacking it would eat
// legitimate answers.
func TestAgent_JSONReplyWithoutToolNameStaysAReply(t *testing.T) {
	answer := `Here are your macros: {"protein": 180, "carbs": 220, "name": "tuesday"}`
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Content: answer}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "garmin__hrv_get"}}},
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "macros?"})
	if err != nil {
		t.Fatal(err)
	}
	if got != answer {
		t.Fatalf("reply = %q, want it delivered untouched", got)
	}
}
