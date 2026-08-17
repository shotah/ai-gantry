package mcpenable

import (
	"testing"

	"github.com/shotah/ai-gantry/internal/provider"
)

func TestMatches(t *testing.T) {
	cases := []struct {
		name, key string
		want      bool
	}{
		{"google__calendar_list_events", "google", true},
		{"google__calendar_list_events", "google__calendar", true},
		{"google__gmail_search_messages", "google__calendar", false},
		{"google__calendar_list_events", "go", false},
		{"google__calendar_list_events", "google__cal", false},
		{"garmin__sleep_get", "garmin__sleep", true},
		{"garmin__sleep_get", "garmin", true},
		{"flights__offers_search", "flights", true},
		{"mcp_enable", "mcp", false},
	}
	for _, tc := range cases {
		if got := Matches(tc.name, tc.key); got != tc.want {
			t.Errorf("Matches(%q, %q) = %v want %v", tc.name, tc.key, got, tc.want)
		}
	}
}

func TestIndex(t *testing.T) {
	defs := []provider.ToolDef{
		{Name: "google__calendar_list_events"},
		{Name: "google__gmail_search_messages"},
		{Name: "garmin__sleep_get"},
		{Name: "flights__offers_search"},
		{Name: "memory_store"},
	}
	got := Index(defs)
	want := map[string]bool{
		"google": true, "google__calendar": true, "google__gmail": true,
		"garmin": true, "flights": true,
	}
	if len(got) != len(want) {
		t.Fatalf("index=%v", got)
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected %q in %v", k, got)
		}
	}
	if !HasSubprefixes("google", got) {
		t.Fatal("google should have subprefixes")
	}
	if HasSubprefixes("flights", got) {
		t.Fatal("flights should not have subprefixes")
	}
}

func TestAlwaysOn(t *testing.T) {
	if !AlwaysOn("memory_store") || !AlwaysOn("mcp_enable") || !AlwaysOn("self_note") {
		t.Fatal("builtins should be always-on")
	}
	if AlwaysOn("google__calendar_list_events") {
		t.Fatal("MCP tools are not always-on")
	}
}
