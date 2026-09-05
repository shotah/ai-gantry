// Package slash is the harness command catalog.
//
// Telegram setMyCommands, stdio's ready line, /help, and the pendant
// mailbox cmds frame all read this list. Handle still implements each
// command; aliases (/clear, /stop, /quit) stay out of the menu.
package slash

import (
	"fmt"
	"strings"
)

// Command is one menu entry. Name is without the leading slash.
type Command struct {
	Name string `json:"name"`
	Hint string `json:"hint"`
	Args bool   `json:"args,omitempty"`
}

// Catalog is the menu the mouths publish. Order is the picker order.
func Catalog() []Command {
	return []Command{
		{Name: "new", Hint: "reset this session's history"},
		{Name: "cancel", Hint: "stop the in-flight turn"},
		{Name: "status", Hint: "uptime, model, history, tools, turns"},
		{Name: "tools", Hint: "prefixed tool catalog (published vs available)"},
		{Name: "examples", Hint: "capability idea (on|off)", Args: true},
		{Name: "perf", Hint: "last turns: invocations, tools, batch, recoveries"},
		{Name: "memstats", Hint: "memory row counts and consolidation"},
		{Name: "toolstats", Hint: "per-tool call ledger since boot"},
		{Name: "tokens", Hint: "prompt token breakdown (estimates)"},
		{Name: "auth", Hint: "remote OAuth (url / paste code)", Args: true},
		{Name: "help", Hint: "this list"},
		{Name: "brief", Hint: "hold a prefix ~6h", Args: true},
		{Name: "short", Hint: "hold a prefix ~27h", Args: true},
		{Name: "off", Hint: "drop a prefix hold", Args: true},
		{Name: "spark", Hint: "looking-after-you wakes (on|off|qty)", Args: true},
		{Name: "engagement", Hint: "same as /spark", Args: true},
	}
}

// HelpText is /help. Same strings as Catalog.
func HelpText() string {
	cmds := Catalog()
	var b strings.Builder
	for i, c := range cmds {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "/%s — %s", c.Name, c.Hint)
	}
	return b.String()
}

// ReadyLine is the stdio banner (catalog plus REPL /quit).
func ReadyLine() string {
	cmds := Catalog()
	parts := make([]string, 0, len(cmds)+1)
	for _, c := range cmds {
		parts = append(parts, "/"+c.Name)
	}
	parts = append(parts, "/quit")
	return "gantry stdio ready — " + strings.Join(parts, " ")
}
