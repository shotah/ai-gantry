package websearch_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/websearch"
)

type stubRunner struct{ defs []provider.ToolDef }

func (r stubRunner) Tools() []provider.ToolDef { return r.defs }

func (r stubRunner) ToolCount() int { return len(r.defs) }

func (r stubRunner) Call(context.Context, string, json.RawMessage) (string, error) {
	return "other-ok", nil
}

func TestToolDefs(t *testing.T) {
	defs := websearch.ToolDefs()
	if len(defs) != 1 || defs[0].Name != websearch.ToolName {
		t.Fatalf("defs = %+v", defs)
	}
	if !strings.Contains(defs[0].Description, "web_search") {
		t.Fatalf("description = %q", defs[0].Description)
	}
}

func TestComposite_MergesAndRoutes(t *testing.T) {
	c := websearch.Composite{
		Other: stubRunner{defs: []provider.ToolDef{{Name: "demo__echo"}}},
	}
	defs := c.Tools()
	if len(defs) != 2 || defs[0].Name != websearch.ToolName || defs[1].Name != "demo__echo" {
		t.Fatalf("defs = %+v", defs)
	}
	if c.ToolCount() != 2 {
		t.Fatalf("ToolCount = %d", c.ToolCount())
	}
	if out, err := c.Call(context.Background(), "demo__echo", nil); err != nil || out != "other-ok" {
		t.Fatalf("route to other: %q %v", out, err)
	}
	if _, err := c.Call(context.Background(), websearch.ToolName, json.RawMessage(`{"query":"x"}`)); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("empty search client: %v", err)
	}
	if _, err := c.Call(context.Background(), "google-search__web_search", json.RawMessage(`{"query":"x"}`)); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("alias should route to builtin: %v", err)
	}
}
