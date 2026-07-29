package mcp_test

import (
	"testing"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

func TestEstimateToolSchemaTokens(t *testing.T) {
	n := mcp.EstimateToolSchemaTokens([]provider.ToolDef{
		{
			Name:        "demo__echo",
			Description: "echo back",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	})
	if n < 1 {
		t.Fatalf("est=%d", n)
	}
}

func TestEstimateSchemaBudget_ByServer(t *testing.T) {
	defs := []provider.ToolDef{
		{Name: "garmin__get_sleep", Description: "sleep", Parameters: map[string]any{"type": "object"}},
		{Name: "garmin__get_weight", Description: "weight", Parameters: map[string]any{"type": "object"}},
		{Name: "memory_store", Description: "store", Parameters: map[string]any{"type": "object"}},
		{Name: "cast__list_local_hardware", Description: "discover", Parameters: map[string]any{"type": "object"}},
	}
	got := mcp.EstimateSchemaBudget(defs)
	if got.Tools != 4 {
		t.Fatalf("tools=%d", got.Tools)
	}
	if got.EstTokens < 1 {
		t.Fatalf("est=%d", got.EstTokens)
	}
	by := map[string]mcp.ServerSchemaBudget{}
	for _, s := range got.ByServer {
		by[s.Server] = s
	}
	if by["garmin"].Tools != 2 || by["cast"].Tools != 1 || by["builtin"].Tools != 1 {
		t.Fatalf("by server = %#v", got.ByServer)
	}
	// Heavier servers first (garmin has two tools).
	if got.ByServer[0].Server != "garmin" {
		t.Fatalf("expected garmin first, got %#v", got.ByServer)
	}
}
