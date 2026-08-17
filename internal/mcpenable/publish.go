package mcpenable

import (
	"strings"

	"github.com/shotah/ai-gantry/internal/provider"
)

// Publish filters the host catalog to always-on builtins, force prefixes, and enabled rows.
func Publish(defs []provider.ToolDef, rows []Row, force Force) []provider.ToolDef {
	var keys []string
	for _, r := range rows {
		keys = append(keys, r.Prefix)
	}
	out := make([]provider.ToolDef, 0, len(defs))
	for _, d := range defs {
		if Allowed(d.Name, keys, force) {
			out = append(out, d)
		}
	}
	return out
}

// Allowed reports whether a tool name may be published or called.
func Allowed(name string, enabled []string, force Force) bool {
	if AlwaysOn(name) || name == ToolName {
		return true
	}
	if force.covers(name) {
		return true
	}
	_, ok := LongestKey(name, enabled)
	return ok
}

// EnableHint is the model-facing error when a real MCP tool is off.
func EnableHint(name string, index []string) string {
	key, ok := LongestKey(name, index)
	if !ok {
		server, _, has := strings.Cut(name, "__")
		if has {
			key = server
		} else {
			key = name
		}
	}
	return "not enabled — call mcp_enable with prefixes=[" + key + "] then retry"
}
