package mcp

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/shotah/ai-gantry/internal/provider"
)

// EstimateToolSchemaTokens estimates prompt cost of tool definitions (chars/4).
// Labeled estimate only — not a tokenizer.
func EstimateToolSchemaTokens(defs []provider.ToolDef) int {
	chars := 0
	for _, d := range defs {
		chars += toolDefChars(d)
	}
	return chars / 4
}

func toolDefChars(d provider.ToolDef) int {
	chars := len(d.Name) + len(d.Description)
	if d.Parameters != nil {
		b, err := json.Marshal(d.Parameters)
		if err == nil {
			chars += len(b)
		}
	}
	return chars
}

// ServerSchemaBudget is the estimated schema cost for one server prefix
// (or "builtin" for unprefixed tools like memory_*/cron_*).
type ServerSchemaBudget struct {
	Server    string
	Tools     int
	EstTokens int
}

// SchemaBudget groups published tool schemas by server for operator visibility
// (/tools, boot logs). EstTokens is chars/4 across all defs.
type SchemaBudget struct {
	Tools     int
	EstTokens int
	ByServer  []ServerSchemaBudget
}

// EstimateSchemaBudget returns total + per-server schema estimates.
func EstimateSchemaBudget(defs []provider.ToolDef) SchemaBudget {
	type acc struct {
		tools int
		chars int
	}
	by := map[string]*acc{}
	totalChars := 0
	for _, d := range defs {
		server := "builtin"
		if i := strings.Index(d.Name, "__"); i > 0 {
			server = d.Name[:i]
		}
		chars := toolDefChars(d)
		totalChars += chars
		a := by[server]
		if a == nil {
			a = &acc{}
			by[server] = a
		}
		a.tools++
		a.chars += chars
	}
	out := SchemaBudget{
		Tools:     len(defs),
		EstTokens: totalChars / 4,
		ByServer:  make([]ServerSchemaBudget, 0, len(by)),
	}
	for server, a := range by {
		out.ByServer = append(out.ByServer, ServerSchemaBudget{
			Server:    server,
			Tools:     a.tools,
			EstTokens: a.chars / 4,
		})
	}
	sort.Slice(out.ByServer, func(i, j int) bool {
		if out.ByServer[i].EstTokens != out.ByServer[j].EstTokens {
			return out.ByServer[i].EstTokens > out.ByServer[j].EstTokens
		}
		return out.ByServer[i].Server < out.ByServer[j].Server
	})
	return out
}
