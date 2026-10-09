package memory

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const todoTextMax = 240

// Todo priority is a marker leading the words (docs/tasks.md §3): "!!"
// urgent, "!" high, nothing normal. The agent writes it with memory_store
// (a same-subject rewrite changes it); the kernel reads it to order the
// stamp, /todo, and the phone frame. No schema: the row is still the words.
const (
	TodoPriorityNormal = 0
	TodoPriorityHigh   = 1
	TodoPriorityUrgent = 2
)

// TodoPriority splits the leading marker from the words. Three or more
// "!" read as urgent.
func TodoPriority(content string) (level int, text string) {
	s := strings.TrimSpace(content)
	n := 0
	for n < len(s) && s[n] == '!' {
		n++
	}
	level = n
	if level > TodoPriorityUrgent {
		level = TodoPriorityUrgent
	}
	return level, strings.TrimSpace(s[n:])
}

// TodoMarker is the marker for a level; "" for normal or out of range.
func TodoMarker(level int) string {
	switch level {
	case TodoPriorityHigh:
		return "!"
	case TodoPriorityUrgent:
		return "!!"
	}
	return ""
}

// TodoWords is the content normalised for a text view: marker, one space,
// the words. The marker stays visible so the agent reading [todo] sees it.
func TodoWords(content string) string {
	level, text := TodoPriority(content)
	m := TodoMarker(level)
	switch {
	case m == "":
		return text
	case text == "":
		return m
	}
	return m + " " + text
}

// SortTodo orders urgent, then high, then normal; inside a level oldest
// first (SortOldestFirst). The input is untouched.
func SortTodo(entries []Entry) []Entry {
	out := SortOldestFirst(entries)
	sort.SliceStable(out, func(i, j int) bool {
		pi, _ := TodoPriority(out[i].Content)
		pj, _ := TodoPriority(out[j].Content)
		return pi > pj
	})
	return out
}

// todoSlug is the shape the phone keeps (docs/tasks.md §4.4). A row
// whose slug fails it is left out of the frame, not out of memory.
var todoSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// TodoItem is one row of the phone's Tasks drawer. JSON names are the
// phone's parser. At is the local day the row was last written; the
// phone shows the age, the crane does not compute it. Priority is the
// marker's level with the marker stripped from Text; absent when normal
// so rows without one keep the original wire shape.
type TodoItem struct {
	ID       int64  `json:"id"`
	Slug     string `json:"slug"`
	Text     string `json:"text"`
	At       string `json:"at"`
	Priority int    `json:"priority,omitempty"`
}

// TodoBoard renders live todo/ rows for the phone: priority first, then
// oldest, no cap (the stamp is what is capped), text whitespace-collapsed
// and clipped, rows with an empty text or an off-pattern slug dropped.
// Nil loc is UTC.
func TodoBoard(entries []Entry, loc *time.Location) []TodoItem {
	if loc == nil {
		loc = time.UTC
	}
	out := make([]TodoItem, 0, len(entries))
	for _, e := range SortTodo(entries) {
		slug := strings.TrimPrefix(strings.TrimSpace(e.Subject), SubjectTodoPrefix)
		if !todoSlug.MatchString(slug) {
			continue
		}
		level, words := TodoPriority(e.Content)
		text := strings.Join(strings.Fields(words), " ")
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) > todoTextMax {
			text = string([]rune(text)[:todoTextMax])
		}
		item := TodoItem{ID: e.ID, Slug: slug, Text: text, Priority: level}
		if at := entryAt(e); !at.IsZero() {
			item.At = at.In(loc).Format("2006-01-02")
		}
		out = append(out, item)
	}
	return out
}
