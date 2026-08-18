package mcpenable

import (
	"context"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/session"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	sess, err := session.Open(t.TempDir(), 20, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	s, err := OpenDB(sess.DB())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStore_EnableTouchExpire(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	index := []string{"google", "google__calendar", "flights"}

	landed, failed, err := s.Enable(ctx, "telegram:1:1", []string{"google__calendar", "flights"}, HoldShort, SourceAgent, now, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(landed) != 2 || len(failed) != 0 {
		t.Fatalf("landed=%v failed=%v", landed, failed)
	}

	_, failed, err = s.Enable(ctx, "telegram:1:1", []string{"nope"}, HoldShort, SourceAgent, now, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Fatalf("expected unknown prefix, failed=%v", failed)
	}

	if err := s.Touch(ctx, "telegram:1:1", "google__calendar_list_events", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rows, err := s.List(ctx, "telegram:1:1", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var cal Row
	for _, r := range rows {
		if r.Prefix == "google__calendar" {
			cal = r
		}
	}
	if cal.LastUsed.Before(now.Add(30 * time.Minute)) {
		t.Fatalf("touch did not refresh: %+v", cal)
	}

	later := now.Add(ShortIdle + 2*time.Hour)
	if err := s.Expire(ctx, later); err != nil {
		t.Fatal(err)
	}
	rows, err = s.List(ctx, "telegram:1:1", later)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected expire, got %+v", rows)
	}
}

func TestStore_HumanSourceSticks(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	index := []string{"garmin__sleep"}
	if _, _, err := s.Enable(ctx, "s", []string{"garmin__sleep"}, HoldShort, SourceHuman, now, index); err != nil {
		t.Fatal(err)
	}
	if _, failed, err := s.Enable(ctx, "s", []string{"garmin__sleep"}, HoldBrief, SourceAgent, now, index); err != nil {
		t.Fatal(err)
	} else if len(failed) != 0 {
		t.Fatalf("failed=%v", failed)
	}
	rows, err := s.List(ctx, "s", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Source != SourceHuman || rows[0].Hold != HoldBrief {
		t.Fatalf("got %+v", rows)
	}
}

func TestStore_BriefExpiresIn6h(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	index := []string{"flights"}
	if _, _, err := s.Enable(ctx, "s", []string{"flights"}, HoldBrief, SourceAgent, now, index); err != nil {
		t.Fatal(err)
	}
	rows, err := s.List(ctx, "s", now.Add(5*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("still in the afternoon: %+v", rows)
	}
	rows, err = s.List(ctx, "s", now.Add(BriefIdle+time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("brief should drop after 6h: %+v", rows)
	}
}

func TestStore_MigratesLongHoldToShort(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO mcp_enable (session_id, prefix, hold, source, last_used)
		VALUES ('s', 'flights', 'long', 'agent', ?)`, now.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDB(s.db); err != nil {
		t.Fatal(err)
	}
	rows, err := s.List(context.Background(), "s", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Hold != HoldShort {
		t.Fatalf("legacy long should become short: %+v", rows)
	}
}

func TestStore_EnableListCap(t *testing.T) {
	s := testStore(t)
	var prefixes []string
	for i := 0; i < MaxEnableList+1; i++ {
		prefixes = append(prefixes, "x")
	}
	_, _, err := s.Enable(context.Background(), "s", prefixes, HoldShort, SourceAgent, time.Now(), []string{"x"})
	if err == nil {
		t.Fatal("expected list cap error")
	}
}
