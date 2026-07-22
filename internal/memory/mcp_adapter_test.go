package memory_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/provider"
)

type fakeCaller struct {
	calls []string
	out   map[string]string
	err   error
}

func (f *fakeCaller) Call(_ context.Context, toolName string, _ json.RawMessage) (string, error) {
	f.calls = append(f.calls, toolName)
	if f.err != nil {
		return "", f.err
	}
	if s, ok := f.out[toolName]; ok {
		return s, nil
	}
	return "ok", nil
}

func TestMCPAdapter_RoutesPrefixedTools(t *testing.T) {
	ctx := context.Background()
	fc := &fakeCaller{out: map[string]string{
		"mem__memory_store":  "stored id=9 kind=fact subject=\"s\"",
		"mem__memory_recall": "id=9 (fact) s: hello\n",
		"mem__memory_forget": "forgot 1 row(s) matching \"hello\"",
	}}
	a, err := memory.NewMCPAdapter(fc, "mem")
	if err != nil {
		t.Fatal(err)
	}

	e, err := a.Store(ctx, memory.KindFact, "s", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != 9 {
		t.Fatalf("id=%d", e.ID)
	}

	hits, err := a.Recall(ctx, "hello", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != 9 {
		t.Fatalf("hits=%#v", hits)
	}

	n, err := a.ForgetQuery(ctx, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}

	if len(fc.calls) != 3 {
		t.Fatalf("calls=%v", fc.calls)
	}
}

type stubTools struct {
	defs []provider.ToolDef
}

func (s *stubTools) Tools() []provider.ToolDef { return s.defs }

func (s *stubTools) ToolCount() int { return len(s.defs) }

func (s *stubTools) Call(context.Context, string, json.RawMessage) (string, error) {
	return "", nil
}

func TestComposite_HidesMCPMemoryTools(t *testing.T) {
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	other := &stubTools{defs: []provider.ToolDef{
		{Name: "mem__memory_store"},
		{Name: "mem__other_tool"},
	}}
	c := memory.Composite{
		Memory:        memory.Tools{Backend: b},
		Other:         other,
		HideMCPServer: "mem",
	}
	names := map[string]bool{}
	for _, d := range c.Tools() {
		names[d.Name] = true
	}
	if !names[memory.ToolStore] {
		t.Fatal("missing builtin memory_store")
	}
	if names["mem__memory_store"] {
		t.Fatal("mcp memory tool should be hidden")
	}
	if !names["mem__other_tool"] {
		t.Fatal("other mcp tool should remain")
	}
}
