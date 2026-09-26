package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shotah/ai-gantry/internal/memory"
)

const (
	todoUsage = "usage: /todo [done <id|slug>]"
	// todoLongList is where the /todo footer says this is a pocket list.
	todoLongList = 10
)

// parseTodoCommand recognises /todo and /todo@bot with its arguments.
func parseTodoCommand(text string) (args []string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil, false
	}
	cmd := fields[0]
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	if !strings.EqualFold(cmd, "/todo") {
		return nil, false
	}
	return fields[1:], true
}

// handleTodo is the pocket list's text view (docs/tasks.md). The agent
// keeps the rows; the kernel's one write is done, which is what the
// phone's checkbox sends.
func (a *Agent) handleTodo(ctx context.Context, args []string) (string, error) {
	if a.memory == nil {
		return "todo: not configured", nil
	}
	if len(args) == 0 {
		return a.todoList(ctx)
	}
	if len(args) == 2 && strings.EqualFold(args[0], "done") {
		return a.todoDone(ctx, args[1])
	}
	return todoUsage, nil
}

func (a *Agent) todoList(ctx context.Context) (string, error) {
	rows, err := a.memory.ListBySubjectPrefix(ctx, memory.KindFact, memory.SubjectTodoPrefix, 0)
	if err != nil {
		if errors.Is(err, memory.ErrNotSupported) {
			return "todo: not configured", nil
		}
		return "", err
	}
	if len(rows) == 0 {
		return "nothing on the list", nil
	}
	now := a.clockNow()
	var b strings.Builder
	for _, e := range memory.SortOldestFirst(rows) {
		slug := strings.TrimPrefix(strings.TrimSpace(e.Subject), memory.SubjectTodoPrefix)
		fmt.Fprintf(&b, "#%d %s: %s", e.ID, slug, strings.Join(strings.Fields(e.Content), " "))
		if age := memory.HorizonAge(e, now); age != "" {
			b.WriteString(" " + age)
		}
		b.WriteByte('\n')
	}
	if len(rows) > todoLongList {
		fmt.Fprintf(&b, "%d open — a pocket list; prune, or use a tracker\n", len(rows))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// todoDone forgets one row by id or slug. A row the kernel no longer has
// is not an error: the agent rewrote it between the phone's paint and the
// tap, and the next board push fixes the drawer.
func (a *Agent) todoDone(ctx context.Context, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return todoUsage, nil
	}
	gone := fmt.Sprintf("todo: %s is gone — the list was updated", todoLabel(key))
	if id, err := strconv.ParseInt(key, 10, 64); err == nil {
		e, err := a.memory.Get(ctx, id)
		if err != nil || e.SupersededBy != nil || !strings.HasPrefix(e.Subject, memory.SubjectTodoPrefix) {
			return gone, nil
		}
		if err := a.memory.Forget(ctx, id); err != nil {
			return "", err
		}
		return fmt.Sprintf("done: #%d %s", e.ID, strings.TrimPrefix(e.Subject, memory.SubjectTodoPrefix)), nil
	}
	slug := strings.ToLower(strings.TrimPrefix(key, memory.SubjectTodoPrefix))
	e, ok, err := a.memory.ActiveByKindSubject(ctx, memory.KindFact, memory.SubjectTodoPrefix+slug)
	if err != nil {
		if errors.Is(err, memory.ErrNotSupported) {
			return "todo: not configured", nil
		}
		return "", err
	}
	if !ok {
		return gone, nil
	}
	if err := a.memory.Forget(ctx, e.ID); err != nil {
		return "", err
	}
	return fmt.Sprintf("done: #%d %s", e.ID, slug), nil
}

func todoLabel(key string) string {
	if _, err := strconv.ParseInt(key, 10, 64); err == nil {
		return "#" + key
	}
	return key
}
