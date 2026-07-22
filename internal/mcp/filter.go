package mcp

import (
	"path"
	"strings"
)

// filterTools applies optional allowlist (tools) then denylist (exclude).
// Patterns in exclude use path.Match shell-style wildcards (*, ?).
func filterTools(spec ServerSpec, tools []Tool) (kept []Tool, before int) {
	before = len(tools)
	allow := make(map[string]struct{}, len(spec.Tools))
	for _, name := range spec.Tools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		allow[name] = struct{}{}
	}
	kept = make([]Tool, 0, len(tools))
	for _, t := range tools {
		name := t.OriginalName
		if len(allow) > 0 {
			if _, ok := allow[name]; !ok {
				continue
			}
		}
		if matchExclude(spec.Exclude, name) {
			continue
		}
		kept = append(kept, t)
	}
	return kept, before
}

func matchExclude(patterns []string, name string) bool {
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == name {
			return true
		}
		ok, err := path.Match(p, name)
		if err == nil && ok {
			return true
		}
	}
	return false
}

func prefixFor(spec ServerSpec) string {
	if p := strings.TrimSpace(spec.ToolsPrefix); p != "" {
		return p
	}
	return spec.Name
}
