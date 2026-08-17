package mcpenable

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// FormatIndex is a byte-stable prompt line: sorted keys, no ages.
// The set only changes on enable/drop/force, so providers can cache it.
func FormatIndex(rows []Row, index []string, force Force) string {
	active, idle := indexParts(rows, index, force, time.Time{})
	return renderIndex(active, idle)
}

// FormatIndexStatus is for /tools — same order, with idle ages.
func FormatIndexStatus(rows []Row, index []string, force Force, now time.Time) string {
	active, idle := indexParts(rows, index, force, now)
	return renderIndex(active, idle)
}

func renderIndex(active, idle []string) string {
	if len(active) == 0 && len(idle) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[mcp prefixes] enable with mcp_enable. ")
	if len(active) > 0 {
		b.WriteString("on: " + strings.Join(active, "; "))
	} else {
		b.WriteString("on: (none)")
	}
	if len(idle) > 0 {
		b.WriteString(" | off: " + strings.Join(idle, ", "))
	}
	return b.String()
}

func indexParts(rows []Row, index []string, force Force, now time.Time) (active, idle []string) {
	on := map[string]Row{}
	for _, r := range rows {
		on[r.Prefix] = r
	}
	type item struct{ key, label string }
	var items []item
	seen := map[string]bool{}
	for _, p := range force.Prefixes {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		items = append(items, item{p, p + " (force)"})
	}
	for _, r := range rows {
		if seen[r.Prefix] || forceCoversKey(force, r.Prefix) {
			continue
		}
		seen[r.Prefix] = true
		label := r.Prefix + " (" + r.Hold + ")"
		if !now.IsZero() {
			age := now.Sub(r.LastUsed).Round(time.Minute)
			if age < 0 {
				age = 0
			}
			label = fmt.Sprintf("%s (%s, %s ago)", r.Prefix, r.Hold, age)
		}
		items = append(items, item{r.Prefix, label})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].key < items[j].key })
	for _, it := range items {
		active = append(active, it.label)
	}
	for _, k := range index {
		if seen[k] || force.covers(k) || forceCoversKey(force, k) {
			continue
		}
		idle = append(idle, k)
	}
	// index is already sorted; idle walks it in order
	return active, idle
}

func forceCoversKey(force Force, key string) bool {
	for _, p := range force.Prefixes {
		if p == key || strings.HasPrefix(key, p+"__") || strings.HasPrefix(p, key+"__") {
			return true
		}
	}
	return false
}
