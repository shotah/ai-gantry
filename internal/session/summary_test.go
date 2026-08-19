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
	got  provider.Request
}

func (f *foldCompleter) Complete(_ context.Context, req provider.Request) (*provider.Result, error) {
	f.got = req
	return &provider.Result{Content: f.body}, nil
}

func TestStore_RollingSummaryOnTrim(t *testing.T) {
	ctx := context.Background()
	store, err := session.Open(t.TempDir(), 4, 100000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var hookedPrior, hookedNext string
	store.WithSummarizer(&session.LLMSummarizer{Completer: &foldCompleter{
		body: "Facts: chris likes espresso\nVoice: gag: \"that gull had a mortgage\"",
	}})
	store.WithFoldHook(func(prior, next string) {
		hookedPrior, hookedNext = prior, next
	})

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
	if !strings.Contains(hookedNext, "gull") {
		t.Fatalf("fold hook not called with new voice: prior=%q next=%q", hookedPrior, hookedNext)
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

func TestLedgerParts(t *testing.T) {
	facts, voice := session.LedgerParts("Facts: likes espresso\nVoice: dry; gag: \"gull\"")
	if facts != "likes espresso" || !strings.Contains(voice, "gull") {
		t.Fatalf("facts=%q voice=%q", facts, voice)
	}
	facts, voice = session.LedgerParts("unlabeled legacy paragraph")
	if facts != "unlabeled legacy paragraph" || voice != "" {
		t.Fatalf("legacy facts=%q voice=%q", facts, voice)
	}
}

func TestLLMSummarizer_FoldKeepsVoice(t *testing.T) {
	fc := &foldCompleter{body: "Facts: Chris likes espresso.\nVoice: gag: \"that gull had a mortgage.\""}
	sum := &session.LLMSummarizer{Completer: fc}
	got, err := sum.Fold(context.Background(), "prior", []session.Message{
		{Role: session.RoleUser, Content: "remember the seagull?"},
		{Role: session.RoleAssistant, Content: `that gull had a mortgage`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "gull") || !strings.Contains(got, "Facts:") || !strings.Contains(got, "Voice:") {
		t.Fatalf("summary=%q", got)
	}
	if len(fc.got.Messages) < 1 {
		t.Fatal("summarizer sent no messages")
	}
	sys := fc.got.Messages[0].Content
	for _, want := range []string{"Facts:", "Voice:", "UNCHANGED", "paraphrased joke is a dead joke", "SELF.md", "8–12", "Keep up to 8"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("summarize prompt missing %q:\n%s", want, sys)
		}
	}
	if strings.Contains(sys, "one tight paragraph.") && !strings.Contains(sys, "Facts:") {
		t.Fatal("old paragraph-only instruction still present")
	}
}

func TestVoiceDelta_NewJokeOnly(t *testing.T) {
	prior := "Facts: espresso\nVoice: dry; gag: \"that gull had a mortgage\""
	next := "Facts: espresso\nVoice: dry; gag: \"that gull had a mortgage\"; he is \"Chef\" this week"
	got := session.VoiceDelta(prior, next)
	if !strings.Contains(got, "Chef") || strings.Contains(got, "gull") || strings.Contains(got, "dry") {
		t.Fatalf("delta=%q", got)
	}
	if session.VoiceDelta(next, next) != "" {
		t.Fatal("unchanged voice must be empty")
	}
	if session.VoiceDelta(prior, "Facts: espresso\nVoice: dry today") != "" {
		t.Fatal("mood weather must not graduate")
	}
}

func TestLLMSummarizer_PreservesPriorVoiceWhenModelDropsIt(t *testing.T) {
	prior := "Facts: likes espresso\nVoice: dry; gag: \"that gull had a mortgage\""
	fc := &foldCompleter{body: "Chris asked about Tuesday calendar."}
	sum := &session.LLMSummarizer{Completer: fc}
	got, err := sum.Fold(context.Background(), prior, []session.Message{
		{Role: session.RoleUser, Content: "what's Tuesday?"},
		{Role: session.RoleAssistant, Content: "checking the calendar"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Facts:") || !strings.Contains(got, "Tuesday") {
		t.Fatalf("facts lost: %q", got)
	}
	if !strings.Contains(got, "Voice:") || !strings.Contains(got, "gull") {
		t.Fatalf("prior voice not copied forward: %q", got)
	}
}
