package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/provider"
)

func TestCollapseOldToolCallArgs(t *testing.T) {
	fat := `{"origin":"37.4,-122.1","destination":"37.8,-122.4"}` + strings.Repeat("x", 200)
	raw := json.RawMessage(`{"id":"c1","type":"function","function":{"name":"maps__route_eta","arguments":"` +
		strings.ReplaceAll(fat, `"`, `\"`) +
		`"},"extra_content":{"google":{"thought_signature":"sig-keep"}}}`)

	orig := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c1", Name: "maps__route_eta", Arguments: fat, Raw: raw},
		}},
		{Role: provider.RoleTool, ToolCallID: "c1", Content: strings.Repeat("R", 800)},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c2", Name: "maps__place_search", Arguments: `{"query":"coffee"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c2", Content: "ok"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c3", Name: "math__expression_evaluate", Arguments: `{"expression":"1+1"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c3", Content: "2"},
	}
	// Keep a copy of the original args so we can prove the input is not mutated.
	origArgs := orig[0].ToolCalls[0].Arguments

	out := collapseOldToolResults(orig)
	if orig[0].ToolCalls[0].Arguments != origArgs {
		t.Fatal("collapse mutated the caller's ToolCalls")
	}
	if !strings.HasPrefix(out[1].Content, "[tool maps__route_eta:") {
		t.Fatalf("old result not collapsed: %q", out[1].Content)
	}
	if out[0].ToolCalls[0].Arguments != collapsedToolArgs {
		t.Fatalf("old args = %q, want stub", out[0].ToolCalls[0].Arguments)
	}
	if !strings.Contains(string(out[0].ToolCalls[0].Raw), "sig-keep") {
		t.Fatalf("thought_signature dropped: %s", out[0].ToolCalls[0].Raw)
	}
	if strings.Contains(string(out[0].ToolCalls[0].Raw), "37.4") {
		t.Fatalf("fat args still in Raw: %s", out[0].ToolCalls[0].Raw)
	}
	if out[2].ToolCalls[0].Arguments != `{"query":"coffee"}` {
		t.Fatalf("recent args collapsed: %q", out[2].ToolCalls[0].Arguments)
	}
	if out[4].ToolCalls[0].Arguments != `{"expression":"1+1"}` {
		t.Fatalf("newest args collapsed: %q", out[4].ToolCalls[0].Arguments)
	}
}
