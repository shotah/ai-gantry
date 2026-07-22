package session_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/session"
)

func TestStore_AppendTrimReset(t *testing.T) {
	dir := t.TempDir()
	store, err := session.Open(dir, 4, 100000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	id := "telegram:1:2"

	for i := 0; i < 3; i++ {
		if err := store.Append(ctx, id,
			session.Message{Role: session.RoleUser, Content: "u"},
			session.Message{Role: session.RoleAssistant, Content: "a"},
		); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	msgs, err := store.Messages(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("len=%d want 4 (trimmed to maxMessages)", len(msgs))
	}

	n, est, err := store.Stats(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 || est <= 0 {
		t.Fatalf("Stats = %d, %d", n, est)
	}

	if err := store.Reset(ctx, id); err != nil {
		t.Fatal(err)
	}
	msgs, err = store.Messages(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("after reset len=%d", len(msgs))
	}

	if _, err := os.Stat(filepath.Join(dir, "gantry.db")); err != nil {
		t.Fatalf("db file missing: %v", err)
	}
}

func TestStore_OpenDefaultsAndEdges(t *testing.T) {
	dir := t.TempDir()
	store, err := session.Open(dir, 0, 0) // defaults
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	if err := store.Append(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(ctx, "s", session.Message{Role: "system", Content: "nope"}); err == nil {
		t.Fatal("expected invalid role error")
	}
	var nilStore *session.Store
	if err := nilStore.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStore_TokenTrim(t *testing.T) {
	dir := t.TempDir()
	// Tiny token budget forces trim even with high message cap.
	store, err := session.Open(dir, 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	big := strings.Repeat("x", 80) // ~20 est tokens each
	for i := 0; i < 5; i++ {
		if err := store.Append(ctx, "s",
			session.Message{Role: session.RoleUser, Content: big},
			session.Message{Role: session.RoleAssistant, Content: big},
		); err != nil {
			t.Fatal(err)
		}
	}
	msgs, err := store.Messages(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) > 4 {
		t.Fatalf("expected token trim, got %d messages", len(msgs))
	}
	if session.EstTokens(msgs) > 10 && len(msgs) > 2 {
		t.Fatalf("est_tokens=%d still over budget with %d msgs", session.EstTokens(msgs), len(msgs))
	}
}
