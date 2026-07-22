package heartbeat_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/heartbeat"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestHeartbeat_TouchAndCheck(t *testing.T) {
	ctx := context.Background()
	store, err := session.Open(t.TempDir(), 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	hb, err := heartbeat.OpenDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	if err := hb.Check(ctx, time.Minute); err == nil {
		t.Fatal("expected missing heartbeat error")
	}
	if err := hb.Touch(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	if err := hb.Check(ctx, time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFile(t *testing.T) {
	dir := t.TempDir()
	store, err := session.Open(dir, 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := heartbeat.OpenDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	if err := hb.Touch(context.Background(), "v"); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	if err := heartbeat.CheckFile(dir, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := heartbeat.CheckFile(filepath.Join(dir, "missing"), time.Minute); err == nil {
		t.Fatal("expected missing db error")
	}
}
