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

	_, failed, err = s.Enable(ctx, "telegram:1:1", []string{"google"}, HoldLong, SourceAgent, now, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Fatalf("expected fat-server refuse, failed=%v", failed)
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

func TestStore_HumanShortBlocksAgentLong(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	index := []string{"garmin__sleep"}
	if _, _, err := s.Enable(ctx, "s", []string{"garmin__sleep"}, HoldShort, SourceHuman, now, index); err != nil {
		t.Fatal(err)
	}
	_, failed, err := s.Enable(ctx, "s", []string{"garmin__sleep"}, HoldLong, SourceAgent, now, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Fatalf("failed=%v", failed)
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
