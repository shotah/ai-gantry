package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Budget caps how many times one server may be called per period. It is the
// human's quota written down — "one rental search a day", "50 flight
// lookups a month" — and it is enforced in the host so every path to the
// server pays it: an agent turn, a spark wake, a watch poll, a retry.
// A model that searches three times for one question, or a watch left on
// the 15-minute default, cannot spend past it.
type Budget struct {
	Calls  int
	Period string // "day" | "month"
}

// Budget periods: a day rolls at the human's midnight, a month on their first.
const (
	BudgetPeriodDay   = "day"
	BudgetPeriodMonth = "month"
)

// ParseBudget reads the mcp.toml `budget` value: "N/day" or "N/month".
// Empty means no budget (ok=false, err=nil).
func ParseBudget(s string) (Budget, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Budget{}, false, nil
	}
	n, period, found := strings.Cut(s, "/")
	if !found {
		return Budget{}, false, fmt.Errorf("mcp: budget %q: want N/day or N/month", s)
	}
	calls, err := strconv.Atoi(strings.TrimSpace(n))
	if err != nil || calls < 1 {
		return Budget{}, false, fmt.Errorf("mcp: budget %q: calls must be a positive integer", s)
	}
	period = strings.ToLower(strings.TrimSpace(period))
	switch period {
	case BudgetPeriodDay, BudgetPeriodMonth:
	default:
		return Budget{}, false, fmt.Errorf("mcp: budget %q: period must be day or month", s)
	}
	return Budget{Calls: calls, Period: period}, true, nil
}

func (b Budget) String() string { return fmt.Sprintf("%d/%s", b.Calls, b.Period) }

// PeriodKey is the counter bucket for now in the human's zone: the date for
// a daily budget, the month for a monthly one.
func (b Budget) PeriodKey(now time.Time) string {
	if b.Period == BudgetPeriodMonth {
		return now.Format("2006-01")
	}
	return now.Format("2006-01-02")
}

// ResetAt is when the next bucket opens: local midnight, or the first of
// next month at local midnight.
func (b Budget) ResetAt(now time.Time) time.Time {
	y, m, d := now.Date()
	if b.Period == BudgetPeriodMonth {
		return time.Date(y, m+1, 1, 0, 0, 0, 0, now.Location())
	}
	return time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
}

// Floor is the shortest poll interval that cannot outrun the budget: the
// period spread evenly over the calls, rounded up to the minute. A watch
// on a 1/day server polls no faster than every 24h.
func (b Budget) Floor() time.Duration {
	period := 24 * time.Hour
	if b.Period == BudgetPeriodMonth {
		period = 30 * 24 * time.Hour
	}
	per := period / time.Duration(b.Calls)
	return per.Round(time.Minute)
}

// BudgetError is the refusal handed back as the tool result. It tells the
// model not to retry and the watch runner when to come back.
type BudgetError struct {
	Server  string
	Budget  Budget
	Used    int
	ResetAt time.Time
}

func (e *BudgetError) Error() string {
	return fmt.Sprintf("mcp: %s budget %d/%s used (%d calls this %s); resets %s — do not retry; use what you already have and tell the human",
		e.Server, e.Budget.Calls, e.Budget.Period, e.Used, e.Budget.Period, e.ResetAt.Format("2006-01-02 15:04 MST"))
}

// RetryAfter is when the budget opens again. The watch runner defers a
// refused poll to this instant instead of polling into the wall.
func (e *BudgetError) RetryAfter() time.Time { return e.ResetAt }

// BudgetStore counts calls per server per period. Take records one call
// when used < limit and reports the count after; ok=false means the cap is
// reached and nothing was recorded. It must be atomic under parallel tool
// batches.
type BudgetStore interface {
	Take(ctx context.Context, server, period string, limit int) (used int, ok bool, err error)
}

// memBudgetStore is the process-lifetime default when no SQLite store is
// wired (tests, single-shot CLIs). It forgets on restart.
type memBudgetStore struct {
	mu    sync.Mutex
	calls map[string]int
}

func newMemBudgetStore() *memBudgetStore { return &memBudgetStore{calls: map[string]int{}} }

func (m *memBudgetStore) Take(_ context.Context, server, period string, limit int) (int, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := server + "\x00" + period
	used := m.calls[key]
	if used >= limit {
		return used, false, nil
	}
	m.calls[key] = used + 1
	return used + 1, true, nil
}

// takeBudget is the host-side gate. No budget for the server → allowed.
// A store error fails open with a warning: a broken counter must not take
// every tool down, and the refusal is a courtesy to a quota, not a lock.
func (h *Host) takeBudget(ctx context.Context, server string) error {
	b, ok := h.budgets[server]
	if !ok {
		return nil
	}
	now := time.Now().In(h.loc)
	used, allowed, err := h.budget.Take(ctx, server, b.PeriodKey(now), b.Calls)
	if err != nil {
		h.log.Warn("mcp budget store failed; allowing call", "server", server, "err", err)
		return nil
	}
	if !allowed {
		h.recordBudgetRefused()
		h.log.Warn("mcp budget refused", "server", server, "budget", b.String(), "used", used)
		return &BudgetError{Server: server, Budget: b, Used: used, ResetAt: b.ResetAt(now)}
	}
	return nil
}

// BudgetFloor is the slowest-safe poll interval for a prefixed tool name,
// zero when its server has no budget. watch_add raises a shorter interval
// to it so a watch cannot be born already over budget. Nil-safe.
func (h *Host) BudgetFloor(toolName string) time.Duration {
	if h == nil {
		return 0
	}
	tool, _, ok := h.resolve(toolName)
	if !ok {
		return 0
	}
	b, ok := h.budgets[tool.Server]
	if !ok {
		return 0
	}
	return b.Floor()
}
