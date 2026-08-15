package watch

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Item is one fetched entry. Id is the cursor key.
type Item struct {
	ID    string
	Title string
	URL   string
	Text  string
}

// ParseItems reads a fetch-tool result. Accepted shapes:
//
//	{"items":[{"id":"...","title":"...","url":"..."}]}
//	{"entries":[...]} / {"data":[...]}
//	[{"id":"..."}]
//
// Id falls back to guid, then url/link. Items without a stable id are dropped.
func ParseItems(raw string) ([]Item, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return itemsFromArray(arr)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, fmt.Errorf("watch: fetch result is not JSON: %w", err)
	}
	for _, key := range []string{"items", "entries", "data", "tweets"} {
		if v, ok := obj[key]; ok {
			list, ok := v.([]any)
			if !ok {
				return nil, fmt.Errorf("watch: %q is not an array", key)
			}
			return itemsFromArray(list)
		}
	}
	return nil, fmt.Errorf("watch: no items array in fetch result")
}

func itemsFromArray(arr []any) ([]Item, error) {
	out := make([]Item, 0, len(arr))
	for _, raw := range arr {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		it := Item{
			ID:    firstString(m, "id", "guid", "ID"),
			Title: firstString(m, "title", "name", "text"),
			URL:   firstString(m, "url", "link", "href"),
			Text:  firstString(m, "summary", "content", "body", "description"),
		}
		if it.ID == "" {
			it.ID = it.URL
		}
		if it.ID == "" {
			continue
		}
		out = append(out, it)
	}
	return out, nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := stringify(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return strings.TrimSpace(fmt.Sprint(x))
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	default:
		return ""
	}
}

// DiffNew returns items whose ids are not in seen, in input order.
func DiffNew(items []Item, seen []string) []Item {
	have := make(map[string]struct{}, len(seen))
	for _, id := range seen {
		have[id] = struct{}{}
	}
	var out []Item
	for _, it := range items {
		if _, ok := have[it.ID]; ok {
			continue
		}
		out = append(out, it)
	}
	return out
}

// MergeSeen appends new ids and keeps the last MaxSeenIDs.
func MergeSeen(seen []string, items []Item) []string {
	have := make(map[string]struct{}, len(seen)+len(items))
	out := make([]string, 0, len(seen)+len(items))
	for _, id := range seen {
		if id == "" {
			continue
		}
		if _, ok := have[id]; ok {
			continue
		}
		have[id] = struct{}{}
		out = append(out, id)
	}
	for _, it := range items {
		if it.ID == "" {
			continue
		}
		if _, ok := have[it.ID]; ok {
			continue
		}
		have[it.ID] = struct{}{}
		out = append(out, it.ID)
	}
	if len(out) > MaxSeenIDs {
		out = out[len(out)-MaxSeenIDs:]
	}
	return out
}

// FormatItems is the untrusted payload handed to the agent on a wake.
func FormatItems(items []Item) string {
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteByte('\n')
		}
		_, _ = fmt.Fprintf(&b, "- id=%s", it.ID)
		if it.Title != "" {
			_, _ = fmt.Fprintf(&b, " title=%q", it.Title)
		}
		if it.URL != "" {
			_, _ = fmt.Fprintf(&b, " url=%s", it.URL)
		}
		if it.Text != "" {
			_, _ = fmt.Fprintf(&b, " text=%q", truncate(it.Text, 240))
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
