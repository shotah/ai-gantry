package memory

import (
	"context"
	"testing"
	"time"
)

func TestBuiltin_searchLike(t *testing.T) {
	b, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	ctx := context.Background()
	if _, err := b.Store(ctx, KindFact, "chris", "likes quiet mornings"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	hits, err := b.searchLike(ctx, "quiet", now, 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
}
