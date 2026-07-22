package memory_test

import (
	"context"
	"strconv"
	"strings"
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

func TestConsolidator_StartAndFencedJSON(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := b.Store(ctx, memory.KindEpisode, "chris", "walks at dawn"); err != nil {
		t.Fatal(err)
	}

	fc := &consolidateCompleter{body: "```json\n[{\"kind\":\"fact\",\"subject\":\"chris\",\"content\":\"walks at dawn\",\"supersedes\":[]}]\n```"}
	c := &memory.Consolidator{
		Store:     b,
		Completer: fc,
		Interval:  15 * time.Millisecond,
		BatchSize: 0, // exercise default batch
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		c.Start(runCtx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not stop")
	}

	// Bad long JSON exercises parse failure + truncate in the warn path.
	if _, err := b.Store(ctx, memory.KindEpisode, "chris", "another episode"); err != nil {
		t.Fatal(err)
	}
	c.Completer = &consolidateCompleter{body: strings.Repeat("not-json-", 40)}
	c.Pass(ctx)
}

func TestTools_ForgetStringID(t *testing.T) {
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	ctx := context.Background()
	e, err := b.Store(ctx, memory.KindFact, "s", "c")
	if err != nil {
		t.Fatal(err)
	}
	tools := memory.Tools{Backend: b}
	out, err := tools.Call(ctx, memory.ToolForget, []byte(`{"id":"`+strconv.FormatInt(e.ID, 10)+`"}`))
	if err != nil || out == "" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
