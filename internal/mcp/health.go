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
	Name  string
	State string
	At    time.Time // zero when idle or skipped
	Tool  string    // last prefixed tool name
	Note  string    // last error (or boot skip reason)
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
	h.mu.RLock()
	names := make([]string, 0, len(h.servers))
	for name := range h.servers {
		names = append(names, name)
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
		row := ServerStatus{Name: name, State: ServerIdle}
		if lc, ok := last[name]; ok {
			row.At = lc.at
			row.Tool = lc.tool
			row.Note = lc.note
			if lc.ok {
				row.State = ServerOK
			} else {
				row.State = ServerError
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
		if r.Note == "" {
			return "skipped"
		}
		return "skipped  " + r.Note
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
