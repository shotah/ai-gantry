package mcpenable

import (
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/provider"
)

func TestPublish(t *testing.T) {
	defs := []provider.ToolDef{
		{Name: "memory_store"},
		{Name: "mcp_enable"},
		{Name: "google__calendar_list_events"},
		{Name: "google__gmail_search_messages"},
		{Name: "flights__offers_search"},
	}
	got := Publish(defs, []Row{{Prefix: "google__calendar"}}, Force{})
	var names []string
	for _, d := range got {
		names = append(names, d.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "memory_store") || !strings.Contains(joined, "mcp_enable") || !strings.Contains(joined, "google__calendar_list_events") {
		t.Fatalf("published=%v", names)
	}
	if strings.Contains(joined, "gmail") || strings.Contains(joined, "flights") {
		t.Fatalf("leaked siblings: %v", names)
	}

	got = Publish(defs, nil, Force{Prefixes: []string{"flights"}})
	names = nil
	for _, d := range got {
		names = append(names, d.Name)
	}
	joined = strings.Join(names, ",")
	if !strings.Contains(joined, "flights__offers_search") {
		t.Fatalf("force flights missing: %v", names)
	}
}

func TestEnableHint(t *testing.T) {
	h := EnableHint("google__calendar_list_events", []string{"google", "google__calendar"})
	if !strings.Contains(h, "mcp_enable") || !strings.Contains(h, "google__calendar") {
		t.Fatalf("hint=%q", h)
	}
}
