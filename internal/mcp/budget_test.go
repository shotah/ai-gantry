package mcp_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestParseBudget(t *testing.T) {
	cases := []struct {
		in    string
		want  mcp.Budget
		ok    bool
		isErr bool
	}{
		{"", mcp.Budget{}, false, false},
		{"1/day", mcp.Budget{Calls: 1, Period: "day"}, true, false},
		{" 50 / Month ", mcp.Budget{Calls: 50, Period: "month"}, true, false},
		{"0/day", mcp.Budget{}, false, true},
		{"-3/day", mcp.Budget{}, false, true},
		{"1/week", mcp.Budget{}, false, true},
		{"daily", mcp.Budget{}, false, true},
		{"x/day", mcp.Budget{}, false, true},
	}
	for _, c := range cases {
		got, ok, err := mcp.ParseBudget(c.in)
		if (err != nil) != c.isErr {
			t.Fatalf("%q: err=%v want isErr=%v", c.in, err, c.isErr)
		}
		if ok != c.ok || got != c.want {
			t.Fatalf("%q: got %+v ok=%v, want %+v ok=%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// The day rolls at the human's midnight, the month on their first — and
// the counter key is what makes two calls "the same day".
func TestBudget_PeriodKeyResetAt(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("tzdata missing")
	}
	// 23:30 on Sep 30 in LA is already Oct 1 in UTC — the key must be LA's.
	now := time.Date(2026, 9, 30, 23, 30, 0, 0, la)
	day := mcp.Budget{Calls: 1, Period: mcp.BudgetPeriodDay}
	if got := day.PeriodKey(now); got != "2026-09-30" {
		t.Fatalf("day key=%q", got)
	}
	if got := day.ResetAt(now); !got.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, la)) {
		t.Fatalf("day reset=%v", got)
	}
	month := mcp.Budget{Calls: 50, Period: mcp.BudgetPeriodMonth}
	if got := month.PeriodKey(now); got != "2026-09" {
		t.Fatalf("month key=%q", got)
	}
	if got := month.ResetAt(now); !got.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, la)) {
		t.Fatalf("month reset=%v", got)
	}
	// December rolls into next year.
	dec := time.Date(2026, 12, 15, 12, 0, 0, 0, la)
	if got := month.ResetAt(dec); !got.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, la)) {
		t.Fatalf("dec reset=%v", got)
	}
}

func TestBudget_Floor(t *testing.T) {
	if got := (mcp.Budget{Calls: 1, Period: "day"}).Floor(); got != 24*time.Hour {
		t.Fatalf("1/day floor=%s", got)
	}
	if got := (mcp.Budget{Calls: 4, Period: "day"}).Floor(); got != 6*time.Hour {
		t.Fatalf("4/day floor=%s", got)
	}
	// 50/month: 30d/50 = 14h24m.
	if got := (mcp.Budget{Calls: 50, Period: "month"}).Floor(); got != 14*time.Hour+24*time.Minute {
		t.Fatalf("50/month floor=%s", got)
	}
}

func startBudgetHost(t *testing.T, budget string, conn *fakeConn) *mcp.Host {
	t.Helper()
	path := writeManifest(t, `
[[server]]
name = "rentals"
command = "unused"
budget = "`+budget+`"

[[server]]
name = "feeds"
command = "unused"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return conn, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	return host
}

// Crystal's case: one search a day. The second call in the same day is
// refused before the child is touched, on Call and CallRaw alike (the
// watch poller uses CallRaw), and the refusal says when to come back.
// A server without a budget is untouched.
func TestHost_BudgetRefusesSecondCallOnEveryPath(t *testing.T) {
	conn := &fakeConn{tools: []mcp.Tool{{OriginalName: "listings_search"}, {OriginalName: "items_list"}}}
	host := startBudgetHost(t, "1/day", conn)
	ctx := context.Background()

	if _, err := host.Call(ctx, "rentals__listings_search", []byte(`{"city":"Denver"}`)); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if conn.calls != 1 {
		t.Fatalf("child calls=%d want 1", conn.calls)
	}

	_, err := host.CallRaw(ctx, "rentals__listings_search", []byte(`{"city":"Denver"}`))
	var be *mcp.BudgetError
	if !errors.As(err, &be) {
		t.Fatalf("second call err=%v, want *BudgetError", err)
	}
	if be.Server != "rentals" || be.Used != 1 || be.Budget.Calls != 1 {
		t.Fatalf("budget error=%+v", be)
	}
	if !be.RetryAfter().After(time.Now()) {
		t.Fatalf("RetryAfter=%v not in the future", be.RetryAfter())
	}
	for _, want := range []string{"rentals budget 1/day used", "do not retry", "resets"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err=%q missing %q", err, want)
		}
	}
	if conn.calls != 1 {
		t.Fatalf("refused call reached the child: calls=%d", conn.calls)
	}
	if got := host.CallStats().BudgetRefused; got != 1 {
		t.Fatalf("BudgetRefused=%d", got)
	}

	// Unbudgeted server: as many as you like.
	for i := 0; i < 3; i++ {
		if _, err := host.Call(ctx, "feeds__items_list", nil); err != nil {
			t.Fatalf("feeds call %d: %v", i, err)
		}
	}
}

// A malformed call must not spend a slot.
func TestHost_BudgetNotSpentOnBadArguments(t *testing.T) {
	conn := &fakeConn{tools: []mcp.Tool{{OriginalName: "listings_search"}}}
	host := startBudgetHost(t, "1/day", conn)
	ctx := context.Background()
	if _, err := host.Call(ctx, "rentals__listings_search", []byte(`{not json`)); err == nil {
		t.Fatal("want arg error")
	}
	if _, err := host.Call(ctx, "rentals__listings_search", []byte(`{}`)); err != nil {
		t.Fatalf("real call after bad args: %v", err)
	}
}

func TestHost_BudgetFloor(t *testing.T) {
	conn := &fakeConn{tools: []mcp.Tool{{OriginalName: "listings_search"}, {OriginalName: "items_list"}}}
	host := startBudgetHost(t, "1/day", conn)
	if got := host.BudgetFloor("rentals__listings_search"); got != 24*time.Hour {
		t.Fatalf("floor=%s", got)
	}
	if got := host.BudgetFloor("feeds__items_list"); got != 0 {
		t.Fatalf("unbudgeted floor=%s", got)
	}
	if got := host.BudgetFloor("nope__thing"); got != 0 {
		t.Fatalf("unknown floor=%s", got)
	}
	var nilHost *mcp.Host
	if got := nilHost.BudgetFloor("rentals__listings_search"); got != 0 {
		t.Fatalf("nil host floor=%s", got)
	}
}

func TestLoadManifest_RejectsBadBudget(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "rentals"
command = "unused"
budget = "once a day"
`)
	_, err := mcp.LoadManifest(path)
	if err == nil || !strings.Contains(err.Error(), `server "rentals"`) || !strings.Contains(err.Error(), "N/day or N/month") {
		t.Fatalf("err=%v", err)
	}
}

// The SQLite store: the count outlives the process, and a parallel batch
// cannot squeeze two calls through the last slot.
func TestBudgetDB_TakeIsAtomicAndPersists(t *testing.T) {
	sess, err := session.Open(t.TempDir(), 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := mcp.OpenBudgetDB(sess.DB())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	const limit = 5
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, err := store.Take(ctx, "rentals", "2026-09", limit)
			if err != nil {
				t.Error(err)
				return
			}
			if ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != limit {
		t.Fatalf("allowed=%d want %d", allowed, limit)
	}
	used, err := store.Used(ctx, "rentals", "2026-09")
	if err != nil || used != limit {
		t.Fatalf("used=%d err=%v", used, err)
	}

	// A new period is a fresh bucket; another server is another row.
	if _, ok, _ := store.Take(ctx, "rentals", "2026-10", limit); !ok {
		t.Fatal("new period should be open")
	}
	if _, ok, _ := store.Take(ctx, "flights", "2026-09", limit); !ok {
		t.Fatal("other server should be open")
	}

	// Reopen on the same handle: the count is still there.
	again, err := mcp.OpenBudgetDB(sess.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := again.Take(ctx, "rentals", "2026-09", limit); ok {
		t.Fatal("count did not persist across open")
	}
	if _, err := mcp.OpenBudgetDB(nil); err == nil {
		t.Fatal("nil db should error")
	}
}
