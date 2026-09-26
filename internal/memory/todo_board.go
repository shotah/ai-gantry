package memory

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const todoTextMax = 240

// todoSlug is the shape the phone keeps (docs/tasks.md §4.4). A row
// whose slug fails it is left out of the frame, not out of memory.
var todoSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// TodoItem is one row of the phone's Tasks drawer. JSON names are the
// phone's parser. At is the local day the row was last written; the
// phone shows the age, the crane does not compute it.
type TodoItem struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Text string `json:"text"`
	At   string `json:"at"`
}

// TodoBoard renders live todo/ rows for the phone: oldest first, no cap
// (the stamp is what is capped), text whitespace-collapsed and clipped,
// rows with an empty text or an off-pattern slug dropped. Nil loc is UTC.
func TodoBoard(entries []Entry, loc *time.Location) []TodoItem {
	if loc == nil {
		loc = time.UTC
	}
	out := make([]TodoItem, 0, len(entries))
	for _, e := range SortOldestFirst(entries) {
		slug := strings.TrimPrefix(strings.TrimSpace(e.Subject), SubjectTodoPrefix)
		if !todoSlug.MatchString(slug) {
			continue
		}
		text := strings.Join(strings.Fields(e.Content), " ")
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) > todoTextMax {
			text = string([]rune(text)[:todoTextMax])
		}
		item := TodoItem{ID: e.ID, Slug: slug, Text: text}
		if at := entryAt(e); !at.IsZero() {
			item.At = at.In(loc).Format("2006-01-02")
		}
		out = append(out, item)
	}
	return out
}
