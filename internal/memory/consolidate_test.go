package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/provider"
)

type consolidateCompleter struct {
	body string
}

func (c *consolidateCompleter) Complete(_ context.Context, _ provider.Request) (*provider.Result, error) {
	return &provider.Result{Content: c.body}, nil
}

func TestConsolidator_ExtractsAndMarks(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	ep, err := b.Store(ctx, memory.KindEpisode, "chris", "likes quiet mornings")
	if err != nil {
		t.Fatal(err)
	}

	fc := &consolidateCompleter{body: `[{"kind":"preference","subject":"chris","content":"likes quiet mornings","supersedes":[]}]`}
	c := &memory.Consolidator{
		Store:     b,
		Completer: fc,
		BatchSize: 10,
	}
	c.Pass(ctx)

	list, err := b.ListUnconsolidatedEpisodes(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("episode %d should be marked consolidated, still %#v", ep.ID, list)
	}

	hits, err := b.Recall(ctx, "quiet", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.Kind == memory.KindPreference && h.Source == memory.SourceConsolidation {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected consolidated preference, got %#v", hits)
	}
}

func TestConsolidator_DisabledInterval(t *testing.T) {
	c := &memory.Consolidator{Interval: 0}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		c.Start(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("disabled consolidator should return immediately")
	}
}
