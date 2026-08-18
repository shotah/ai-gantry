// Package mcpenable filters the published MCP tool schemas per session.
// Prefixes stay off until mcp_enable (or operator force-on). Idle clocks drop them.
package mcpenable

import (
	"sort"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/provider"
)

const (
	// HoldBrief is the 6h idle window (this morning / this afternoon).
	HoldBrief = "brief"
	// HoldShort is the 27h idle window (current job).
	HoldShort = "short"
	// HoldLong is the 76h idle window (weekend-shaped habit).
	HoldLong = "long"

	// BriefIdle covers a morning or afternoon without riding overnight.
	BriefIdle = 6 * time.Hour
	// ShortIdle is a day plus morning slack.
	ShortIdle = 27 * time.Hour
	// LongIdle covers a Friday-to-Monday gap.
	LongIdle = 76 * time.Hour

	// MaxLong caps long-active rows per session.
	MaxLong = 4
	// MaxEnableList caps prefixes in one mcp_enable call.
	MaxEnableList = 8

	// ToolName is the builtin that turns prefixes on.
	ToolName = "mcp_enable"

	// SourceAgent is an mcp_enable call.
	SourceAgent = "agent"
	// SourceHuman is /brief /short /long — the model cannot flip a human hold up to long.
	SourceHuman = "human"
)

// Matches reports whether tool name is covered by key (segment-aware).
// google matches google__*; google__calendar matches google__calendar_*;
// go does not match google__*.
func Matches(name, key string) bool {
	name = strings.TrimSpace(name)
	key = strings.TrimSpace(key)
	if name == "" || key == "" {
		return false
	}
	if name == key {
		return true
	}
	if !strings.HasPrefix(name, key) {
		return false
	}
	rest := name[len(key):]
	if strings.Contains(key, "__") {
		return strings.HasPrefix(rest, "_")
	}
	return strings.HasPrefix(rest, "__")
}

// AlwaysOn reports builtins that never idle-drop (no server__ prefix).
func AlwaysOn(name string) bool {
	return !strings.Contains(name, "__")
}

// Index lists enable keys for a catalog: each server prefix, plus
// server__family when that server has two or more distinct families.
func Index(defs []provider.ToolDef) []string {
	type fams struct {
		set map[string]struct{}
	}
	by := map[string]*fams{}
	for _, d := range defs {
		server, rest, ok := strings.Cut(d.Name, "__")
		if !ok || server == "" || rest == "" {
			continue
		}
		fam, _, _ := strings.Cut(rest, "_")
		if fam == "" {
			continue
		}
		g := by[server]
		if g == nil {
			g = &fams{set: map[string]struct{}{}}
			by[server] = g
		}
		g.set[fam] = struct{}{}
	}
	var out []string
	for server, g := range by {
		out = append(out, server)
		if len(g.set) < 2 {
			continue
		}
		for fam := range g.set {
			out = append(out, server+"__"+fam)
		}
	}
	sort.Strings(out)
	return out
}

// HasSubprefixes reports whether key is a bare server that has family keys in index.
func HasSubprefixes(key string, index []string) bool {
	if strings.Contains(key, "__") {
		return false
	}
	want := key + "__"
	for _, k := range index {
		if strings.HasPrefix(k, want) {
			return true
		}
	}
	return false
}

// LongestKey returns the longest enabled prefix that matches name.
func LongestKey(name string, keys []string) (string, bool) {
	best := ""
	for _, k := range keys {
		if Matches(name, k) && len(k) >= len(best) {
			best = k
		}
	}
	return best, best != ""
}

func normalizeHold(hold string) string {
	switch strings.ToLower(strings.TrimSpace(hold)) {
	case HoldBrief:
		return HoldBrief
	case HoldLong:
		return HoldLong
	default:
		return HoldShort
	}
}
