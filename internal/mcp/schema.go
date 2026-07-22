package mcp

import (
	"encoding/json"

	"github.com/shotah/ai-gantry/internal/provider"
)

// EstimateToolSchemaTokens estimates prompt cost of tool definitions (chars/4).
// Labeled estimate only — not a tokenizer.
func EstimateToolSchemaTokens(defs []provider.ToolDef) int {
	chars := 0
	for _, d := range defs {
		chars += len(d.Name) + len(d.Description)
		if d.Parameters != nil {
			b, err := json.Marshal(d.Parameters)
			if err == nil {
				chars += len(b)
			}
		}
	}
	return chars / 4
}
