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
	foundPref := false
	for _, h := range hits {
		if h.Kind == memory.KindPreference && h.Source == memory.SourceConsolidation {
			foundPref = true
		}
		if h.ID == ep.ID {
			t.Fatalf("consolidated episode %d should not appear in recall", ep.ID)
		}
	}
	if !foundPref {
		t.Fatalf("expected consolidated preference, got %#v", hits)
	}
}

func TestConsolidator_ParseFailDoesNotMark(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	ep, err := b.Store(ctx, memory.KindEpisode, "chris", "keep me")
	if err != nil {
		t.Fatal(err)
	}
	c := &memory.Consolidator{
		Store:     b,
		Completer: &consolidateCompleter{body: strings.Repeat("not-json-", 40)},
		BatchSize: 10,
	}
	c.Pass(ctx)
	list, err := b.ListUnconsolidatedEpisodes(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != ep.ID {
		t.Fatalf("parse fail must leave episode unconsolidated, got %#v", list)
	}
}

func TestConsolidator_EmptyReplyDoesNotMark(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := b.Store(ctx, memory.KindEpisode, "chris", "empty reply case"); err != nil {
		t.Fatal(err)
	}
	c := &memory.Consolidator{
		Store:     b,
		Completer: &consolidateCompleter{body: "   "},
		BatchSize: 10,
	}
	c.Pass(ctx)
	list, err := b.ListUnconsolidatedEpisodes(ctx, 20)
	if err != nil || len(list) != 1 {
		t.Fatalf("empty reply must not mark, list=%#v err=%v", list, err)
	}
}

func TestConsolidator_ExplicitEmptyArrayMarks(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := b.Store(ctx, memory.KindEpisode, "chris", "chitchat only"); err != nil {
		t.Fatal(err)
	}
	c := &memory.Consolidator{
		Store:     b,
		Completer: &consolidateCompleter{body: `[]`},
		BatchSize: 10,
	}
	c.Pass(ctx)
	list, err := b.ListUnconsolidatedEpisodes(ctx, 20)
	if err != nil || len(list) != 0 {
		t.Fatalf("explicit [] should mark consolidated, list=%#v err=%v", list, err)
	}
}

func TestConsolidator_QuarantinesAfterMaxFailures(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	ep, err := b.Store(ctx, memory.KindEpisode, "chris", "poison")
	if err != nil {
		t.Fatal(err)
	}
	c := &memory.Consolidator{
		Store:     b,
		Completer: &consolidateCompleter{body: "not-json"},
		BatchSize: 10,
	}
	for i := 0; i < 3; i++ {
		c.Pass(ctx)
	}
	list, err := b.ListUnconsolidatedEpisodes(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("after max failures episode should quarantine out of list, got %#v", list)
	}
	// Still in DB but not recalled as active episode.
	hits, err := b.Recall(ctx, "poison", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.ID == ep.ID {
			t.Fatalf("quarantined episode should not hydrate/recall")
		}
	}
}

func TestConsolidator_SupersedesOnlyBatchIDs(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	keep, err := b.Store(ctx, memory.KindFact, "chris", "keep this fact")
	if err != nil {
		t.Fatal(err)
	}
	ep, err := b.Store(ctx, memory.KindEpisode, "chris", "new note")
	if err != nil {
		t.Fatal(err)
	}
	body := `[{"kind":"fact","subject":"chris","content":"new note","supersedes":[` +
		strconv.FormatInt(keep.ID, 10) + `,` + strconv.FormatInt(ep.ID, 10) + `]}]`
	c := &memory.Consolidator{
		Store:     b,
		Completer: &consolidateCompleter{body: body},
		BatchSize: 10,
	}
	c.Pass(ctx)
	got, err := b.Recall(ctx, "keep this fact", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range got {
		if h.ID == keep.ID && h.SupersededBy != nil {
			t.Fatal("hallucinated supersede must not hide durable fact outside batch")
		}
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
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		list, err := b.ListUnconsolidatedEpisodes(ctx, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not stop")
	}
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
