package heartbeat_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/heartbeat"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestStore_Start(t *testing.T) {
	store, err := session.Open(t.TempDir(), 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	hb, err := heartbeat.OpenDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hb.Start(ctx, 20*time.Millisecond, "test", slog.Default())
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	if err := hb.Check(context.Background(), time.Minute); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not stop")
	}
}
