package mcpenable

import "strings"

// Force is operator always-on prefixes (env + mcp.toml force = true).
// Not used when dynamic_tools = false (full catalog).
type Force struct {
	Prefixes []string
}

// ParseForceCSV parses MCP_ENABLE_FORCE (comma-separated prefixes).
func ParseForceCSV(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (f Force) covers(name string) bool {
	for _, p := range f.Prefixes {
		if Matches(name, p) {
			return true
		}
	}
	return false
}
