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
