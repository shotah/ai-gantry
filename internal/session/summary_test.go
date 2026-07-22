package session_test

import (
	"context"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

type foldCompleter struct {
	body string
}

func (f *foldCompleter) Complete(_ context.Context, _ provider.Request) (*provider.Result, error) {
	return &provider.Result{Content: f.body}, nil
}

func TestStore_RollingSummaryOnTrim(t *testing.T) {
	ctx := context.Background()
	store, err := session.Open(t.TempDir(), 4, 100000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	store.WithSummarizer(&session.LLMSummarizer{Completer: &foldCompleter{body: "chris likes espresso"}})

	id := "s1"
	for i := 0; i < 3; i++ {
		if err := store.Append(ctx, id,
			session.Message{Role: session.RoleUser, Content: "u"},
			session.Message{Role: session.RoleAssistant, Content: "a"},
		); err != nil {
			t.Fatal(err)
		}
	}
	sum, err := store.Summary(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sum, "espresso") {
		t.Fatalf("summary=%q", sum)
	}

	if err := store.Reset(ctx, id); err != nil {
		t.Fatal(err)
	}
	sum, err = store.Summary(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if sum != "" {
		t.Fatalf("summary should clear on reset, got %q", sum)
	}
}
