package memory_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/provider"
)

func TestIsMemoryToolAndDefs(t *testing.T) {
	if !memory.IsMemoryTool(memory.ToolStore) || memory.IsMemoryTool("x") {
		t.Fatal("IsMemoryTool")
	}
	if len(memory.ToolDefs()) != 3 {
		t.Fatal("ToolDefs")
	}
}

func TestComposite_CallAndCount(t *testing.T) {
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	other := &countTools{defs: []provider.ToolDef{{Name: "a__b"}}}
	c := memory.Composite{
		Memory: memory.Tools{Backend: b},
		Other:  other,
	}
	if c.ToolCount() < 4 {
		t.Fatalf("count=%d", c.ToolCount())
	}
	ctx := context.Background()
	if _, err := c.Call(ctx, memory.ToolStore, json.RawMessage(`{"kind":"fact","subject":"s","content":"c"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call(ctx, "a__b", nil); err != nil {
		t.Fatal(err)
	}
	if other.n != 1 {
		t.Fatalf("other=%d", other.n)
	}
}

func TestMCPAdapter_ForgetHydrateClose(t *testing.T) {
	fc := &fakeCaller{out: map[string]string{
		"mem__memory_forget": "forgot id=1",
		"mem__memory_recall": "id=1 (fact) s: hello",
	}}
	a, err := memory.NewMCPAdapter(fc, "mem")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := a.Forget(ctx, 1); err != nil {
		t.Fatal(err)
	}
	hits, err := a.Hydrate(ctx, "hello", 5)
	if err != nil || len(hits) == 0 {
		t.Fatalf("hydrate=%v err=%v", hits, err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltin_EmptyRecallListsActive(t *testing.T) {
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	ctx := context.Background()
	_, _ = b.Store(ctx, memory.KindFact, "x", "y")
	hits, err := b.Recall(ctx, "", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
}

type countTools struct {
	defs []provider.ToolDef
	n    int
}

func (c *countTools) Tools() []provider.ToolDef { return c.defs }

func (c *countTools) ToolCount() int { return len(c.defs) }

func (c *countTools) Call(context.Context, string, json.RawMessage) (string, error) {
	c.n++
	return "ok", nil
}
