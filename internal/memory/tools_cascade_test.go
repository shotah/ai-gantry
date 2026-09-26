package memory_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/shotah/ai-gantry/internal/memory"
)

func TestForget_CascadesAimArea(t *testing.T) {
	ctx := context.Background()
	mem, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mem.Close() })
	e, err := mem.Store(ctx, memory.KindInsight, "aim/training", "3x gym")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindFact, memory.SubjectAimBootstrap, "asked"); err != nil {
		t.Fatal(err)
	}
	var forgot []string
	tools := memory.Tools{
		Backend: mem,
		ForgetAim: func(_ context.Context, area string) error {
			forgot = append(forgot, area)
			return nil
		},
	}
	if _, err := tools.Call(ctx, memory.ToolForget, []byte(`{"id":`+strconv.FormatInt(e.ID, 10)+`}`)); err != nil {
		t.Fatal(err)
	}
	if len(forgot) != 1 || forgot[0] != "training" {
		t.Fatalf("forgot %v", forgot)
	}

	if _, err := mem.Store(ctx, memory.KindFact, memory.SubjectAimBootstrap, "asked again"); err != nil {
		t.Fatal(err)
	}
	boot, ok, err := mem.ActiveByKindSubject(ctx, memory.KindFact, memory.SubjectAimBootstrap)
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	forgot = nil
	if _, err := tools.Call(ctx, memory.ToolForget, []byte(`{"id":`+strconv.FormatInt(boot.ID, 10)+`}`)); err != nil {
		t.Fatal(err)
	}
	if len(forgot) != 0 {
		t.Fatalf("bootstrap cascaded: %v", forgot)
	}
}
