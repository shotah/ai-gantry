package mcp

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Last-call states for one MCP server this process. Idle means connected
// (tools are in the catalog) but nothing has been called yet.
const (
	ServerIdle    = "idle"
	ServerOK      = "ok"
	ServerError   = "error"
	ServerSkipped = "skipped"
)

const healthNoteMax = 80

// ServerStatus is the last observed call for one manifest server.
type ServerStatus struct {
	Name   string
	State  string
	At     time.Time // zero when idle or skipped
	Tool   string    // last prefixed tool name
	Note   string    // last error (or boot skip reason)
	Reason Reason    // no_binary | no_key | no_oauth | connect; empty when idle/ok
	Auth   bool      // mcp.toml declares auth_args / auth_command
	Prefix string    // tools_prefix or name; used to match model-invented calls
}

// ServerHealthOf forwards ServerHealth through composite tool runners.
func ServerHealthOf(v any) []ServerStatus {
	if s, ok := v.(interface{ ServerHealth() []ServerStatus }); ok {
		return s.ServerHealth()
	}
	return nil
}

// ServerHealth lists every connected server (tools in the catalog) plus
// boot-skipped manifest servers. Sorted by name.
func (h *Host) ServerHealth() []ServerStatus {
	type meta struct {
		auth   bool
		prefix string
	}
	h.mu.RLock()
	names := make([]string, 0, len(h.servers))
	info := make(map[string]meta, len(h.servers))
	for name, ms := range h.servers {
		names = append(names, name)
		info[name] = meta{auth: ms.spec.AuthConfigured(), prefix: prefixFor(ms.spec)}
	}
	skipped := append([]ServerStatus(nil), h.skipped...)
	h.mu.RUnlock()

	h.stats.mu.Lock()
	last := make(map[string]serverLastCall, len(h.stats.byServer))
	for k, v := range h.stats.byServer {
		last[k] = v
	}
	h.stats.mu.Unlock()

	out := make([]ServerStatus, 0, len(names)+len(skipped))
	for _, name := range names {
		row := ServerStatus{Name: name, State: ServerIdle, Auth: info[name].auth, Prefix: info[name].prefix}
		if lc, ok := last[name]; ok {
			row.At = lc.at
			row.Tool = lc.tool
			row.Note = lc.note
			if lc.ok {
				row.State = ServerOK
			} else {
				row.State = ServerError
				row.Reason = ClassifyReason(lc.note)
			}
		}
		out = append(out, row)
	}
	out = append(out, skipped...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// FormatServerHealth renders the compact [tools] prompt block.
// Skipped servers are omitted: they have no published tools, and listing
// them invites the model to invent names. now is used for "Nm ago".
func FormatServerHealth(rows []ServerStatus, now time.Time) string {
	var shown []ServerStatus
	for _, r := range rows {
		if r.State == ServerSkipped {
			continue
		}
		shown = append(shown, r)
	}
	if len(shown) == 0 {
		return ""
	}
	if now.IsZero() {
		now = time.Now()
	}
	width := 0
	for _, r := range shown {
		if n := utf8.RuneCountInString(r.Name); n > width {
			width = n
		}
	}
	if width < 4 {
		width = 4
	}
	var b strings.Builder
	b.WriteString("[tools]\n")
	for _, r := range shown {
		age := "—"
		if !r.At.IsZero() {
			age = formatHealthAge(now.Sub(r.At))
		}
		note := r.Note
		if r.State == ServerOK {
			note = shortToolSuffix(r.Tool)
		} else if r.State == ServerError && r.Tool != "" {
			suf := shortToolSuffix(r.Tool)
			if note != "" {
				note = suf + ": " + note
			} else {
				note = suf
			}
		}
		if note == "" {
			fmt.Fprintf(&b, "%-*s  %-5s  %s\n", width, r.Name, r.State, age)
			continue
		}
		fmt.Fprintf(&b, "%-*s  %-5s  %s  %s\n", width, r.Name, r.State, age, note)
	}
	return strings.TrimRight(b.String(), "\n")
}

// FormatServerHealthLine is one operator line for /tools (includes skipped).
func FormatServerHealthLine(r ServerStatus, now time.Time) string {
	if r.State == ServerSkipped {
		switch {
		case r.Reason != "" && r.Note != "":
			return "skipped  " + string(r.Reason) + "  " + r.Note
		case r.Reason != "":
			return "skipped  " + string(r.Reason)
		case r.Note == "":
			return "skipped"
		default:
			return "skipped  " + r.Note
		}
	}
	if r.State == ServerIdle || r.At.IsZero() {
		return "idle"
	}
	age := formatHealthAge(now.Sub(r.At))
	if r.State == ServerOK {
		if suf := shortToolSuffix(r.Tool); suf != "" {
			return "ok  " + age + "  " + suf
		}
		return "ok  " + age
	}
	note := r.Note
	if suf := shortToolSuffix(r.Tool); suf != "" && note != "" {
		note = suf + ": " + note
	}
	if r.Reason != "" {
		if note == "" {
			return "error  " + age + "  " + string(r.Reason)
		}
		return "error  " + age + "  " + string(r.Reason) + "  " + note
	}
	if note == "" {
		return "error  " + age
	}
	return "error  " + age + "  " + note
}

func formatHealthAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func shortToolSuffix(name string) string {
	_, rest, ok := strings.Cut(name, "__")
	if ok {
		return rest
	}
	return name
}

func clipHealthNote(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) <= healthNoteMax {
		return s
	}
	r := []rune(s)
	return string(r[:healthNoteMax-1]) + "…"
}
