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

// denverBatch is the round-1 batch the Denver eval fixture produces: two
// same-name searches for two possible Fridays plus the calendar, in one
// parallel batch.
func denverBatch() []provider.Message {
	return []provider.Message{
		{Role: provider.RoleUser, Content: "I need to be in Denver next Friday for a client thing."},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c1", Name: "flights__offers_search", Arguments: `{"origin":"SEA","destination":"DEN","outbound_date":"2026-09-25"}`},
			{ID: "c2", Name: "flights__offers_search", Arguments: `{"origin":"SEA","destination":"DEN","outbound_date":"2026-09-18"}`},
			{ID: "c3", Name: "google__calendar_list_events", Arguments: `{"time_min":"2026-09-18T00:00:00-07:00"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c1", Content: `{"offers":[{"airline":"Alaska","price":189}],"date":"09-25"}`},
		{Role: provider.RoleTool, ToolCallID: "c2", Content: `{"offers":[{"airline":"United","price":218}],"date":"09-18"}`},
		{Role: provider.RoleTool, ToolCallID: "c3", Content: `{"events":[]}`},
	}
}

// The batch the model just made is read back whole next round — every
// payload, both same-name searches, arguments intact. This was the eval's
// duplicated paid search and its invented fare: the first result of a
// three-call batch was a "[truncated]" stub before the model ever saw it.
func TestCollapse_UnreadBatchStaysWhole(t *testing.T) {
	msgs := denverBatch()
	out := collapseOldToolResults(msgs)
	for i := 2; i <= 4; i++ {
		if strings.HasPrefix(out[i].Content, "[tool ") {
			t.Fatalf("result %s collapsed before it was read: %q", out[i].ToolCallID, out[i].Content)
		}
	}
	for j, tc := range out[1].ToolCalls {
		if tc.Arguments == collapsedToolArgs {
			t.Fatalf("call %d args stubbed in the unread batch", j)
		}
	}
}

// The window counts rounds, not payloads: a three-call round followed by a
// one-call round is two rounds and stays whole for the reply; a third round
// ages the first one out — all three of its payloads and args together.
func TestCollapse_WindowIsRoundsNotPayloads(t *testing.T) {
	msgs := append(denverBatch(),
		provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c4", Name: "memory_store", Arguments: `{"subject":"event/denver"}`},
		}},
		provider.Message{Role: provider.RoleTool, ToolCallID: "c4", Content: "stored"},
	)
	out := collapseOldToolResults(msgs)
	for _, i := range []int{2, 3, 4, 6} {
		if strings.HasPrefix(out[i].Content, "[tool ") {
			t.Fatalf("two rounds should stay whole; %s collapsed", out[i].ToolCallID)
		}
	}

	msgs = append(msgs,
		provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c5", Name: "cron_schedule", Arguments: `{"in":"2h"}`},
		}},
		provider.Message{Role: provider.RoleTool, ToolCallID: "c5", Content: "scheduled"},
	)
	out = collapseOldToolResults(msgs)
	for _, i := range []int{2, 3, 4} {
		if !strings.HasPrefix(out[i].Content, "[tool ") {
			t.Fatalf("round 1 should age out at round 3; %s kept: %q", out[i].ToolCallID, out[i].Content)
		}
	}
	for j, tc := range out[1].ToolCalls {
		if tc.Arguments != collapsedToolArgs {
			t.Fatalf("aged round's call %d args not stubbed: %q", j, tc.Arguments)
		}
	}
	if out[6].Content != "stored" || out[8].Content != "scheduled" {
		t.Fatalf("last two rounds must stay whole: %q %q", out[6].Content, out[8].Content)
	}
	if msgs[2].Content == out[2].Content {
		t.Fatal("expected a collapsed copy, got the original")
	}
}

// Same tool name across two kept rounds is two different answers (two
// dates), not a stack to squeeze; both stay.
func TestCollapse_SameNameAcrossKeptRoundsStays(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c1", Name: "flights__offers_search", Arguments: `{"outbound_date":"2026-09-25"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c1", Content: `{"date":"09-25"}`},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c2", Name: "flights__offers_search", Arguments: `{"outbound_date":"2026-09-18"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c2", Content: `{"date":"09-18"}`},
	}
	out := collapseOldToolResults(msgs)
	if out[1].Content != `{"date":"09-25"}` || out[3].Content != `{"date":"09-18"}` {
		t.Fatalf("both dates should survive: %q %q", out[1].Content, out[3].Content)
	}
}
