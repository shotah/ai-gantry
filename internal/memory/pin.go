package memory

import (
	"context"
	"strings"
)

const maxPinWalk = 8

// ResolvePin loads the live memory for a cron job: follow superseded_by from
// id, then fall back to subject across durable kinds.
func ResolvePin(ctx context.Context, m Memory, id int64, subject string) (Entry, bool) {
	if m == nil {
		return Entry{}, false
	}
	if id > 0 {
		e, err := m.Get(ctx, id)
		if err == nil {
			e = walkLive(ctx, m, e)
			return e, true
		}
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return Entry{}, false
	}
	for _, k := range []string{KindFact, KindPreference, KindInsight, KindPerson} {
		e, ok, err := m.ActiveByKindSubject(ctx, k, subject)
		if err == nil && ok {
			return e, true
		}
	}
	return Entry{}, false
}

func walkLive(ctx context.Context, m Memory, e Entry) Entry {
	seen := map[int64]struct{}{e.ID: {}}
	cur := e
	for i := 0; i < maxPinWalk; i++ {
		if cur.SupersededBy == nil || *cur.SupersededBy == 0 {
			return cur
		}
		nextID := *cur.SupersededBy
		if _, ok := seen[nextID]; ok {
			return cur
		}
		n, err := m.Get(ctx, nextID)
		if err != nil {
			return cur
		}
		seen[nextID] = struct{}{}
		cur = n
	}
	return cur
}
