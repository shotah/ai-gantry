package memory_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestBuiltin_StoreRecallForget(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	b, err := memory.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	e, err := b.Store(ctx, memory.KindPreference, "chris", "coaching tone, no fluff")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 {
		t.Fatal("expected id")
	}

	hits, err := b.Recall(ctx, "coaching", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Content != e.Content {
		t.Fatalf("recall = %#v", hits)
	}

	if err := b.Forget(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	hits, err = b.Recall(ctx, "coaching", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected empty after forget, got %#v", hits)
	}
}

func TestBuiltin_SurvivesSessionReset(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := session.Open(dir, 50, 8000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	mem, err := memory.OpenDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mem.Store(ctx, memory.KindPerson, "mom", "prefers calls over texts"); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(ctx, "s1",
		session.Message{Role: session.RoleUser, Content: "hi"},
		session.Message{Role: session.RoleAssistant, Content: "hey"},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.Reset(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	msgs, err := store.Messages(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("session should be empty, got %d", len(msgs))
	}

	hits, err := mem.Recall(ctx, "mom", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("memory should survive /new, got %#v", hits)
	}
}

func TestBuiltin_HydrateAndForgetQuery(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	_, _ = b.Store(ctx, memory.KindFact, "climbing", "trains Tue/Thu")
	_, _ = b.Store(ctx, memory.KindEpisode, "today", "skipped gym while traveling")

	hydrated, err := b.Hydrate(ctx, "traveling", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(hydrated) < 1 {
		t.Fatal("expected hydrate rows")
	}
	block := memory.FormatHydration(hydrated, time.UTC)
	if block == "" || block[:8] != "[memory]" {
		t.Fatalf("bad hydration block: %q", block)
	}

	n, err := b.ForgetQuery(ctx, "climbing")
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected forget count >= 1, got %d", n)
	}
}

func TestBuiltin_ConsolidationDedupes(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(filepath.Join(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	ep1, err := b.Store(ctx, memory.KindEpisode, "chris", "likes espresso")
	if err != nil {
		t.Fatal(err)
	}
	ep2, err := b.Store(ctx, memory.KindEpisode, "chris", "drinks espresso every morning")
	if err != nil {
		t.Fatal(err)
	}

	list, err := b.ListUnconsolidatedEpisodes(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("episodes = %d", len(list))
	}

	neu, err := b.StoreConsolidated(ctx, memory.KindPreference, "chris", "drinks espresso every morning")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Supersede(ctx, ep1.ID, neu.ID); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkConsolidated(ctx, []int64{ep1.ID, ep2.ID}); err != nil {
		t.Fatal(err)
	}

	list, err = b.ListUnconsolidatedEpisodes(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no unconsolidated, got %#v", list)
	}

	// Superseded episode should not appear in active recall.
	hits, err := b.Recall(ctx, "espresso", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.ID == ep1.ID {
			t.Fatalf("superseded episode still recalled: %#v", h)
		}
	}
}

func TestBuiltin_StoreSupersedesSameSubject(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	old, err := b.Store(ctx, memory.KindPreference, "pref/food", "dislikes sushi")
	if err != nil {
		t.Fatal(err)
	}
	neu, err := b.Store(ctx, memory.KindPreference, "pref/food", "likes sushi now")
	if err != nil {
		t.Fatal(err)
	}
	if neu.ID == old.ID {
		t.Fatal("expected a new row")
	}
	got, err := b.Get(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SupersededBy == nil || *got.SupersededBy != neu.ID {
		t.Fatalf("old superseded_by=%v want %d", got.SupersededBy, neu.ID)
	}
	live, ok, err := b.ActiveByKindSubject(ctx, memory.KindPreference, "pref/food")
	if err != nil || !ok {
		t.Fatalf("live ok=%v err=%v", ok, err)
	}
	if live.ID != neu.ID || live.Content != "likes sushi now" {
		t.Fatalf("live=%#v", live)
	}
	hydrated, err := b.Hydrate(ctx, "sushi", 30)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range hydrated {
		if e.ID == old.ID {
			t.Fatalf("superseded row still hydrated: %#v", e)
		}
	}
	pin, ok := memory.ResolvePin(ctx, b, old.ID, "")
	if !ok || pin.ID != neu.ID {
		t.Fatalf("ResolvePin should walk to live row, got ok=%v %#v", ok, pin)
	}

	ep1, err := b.Store(ctx, memory.KindEpisode, "chris", "note one")
	if err != nil {
		t.Fatal(err)
	}
	ep2, err := b.Store(ctx, memory.KindEpisode, "chris", "note two")
	if err != nil {
		t.Fatal(err)
	}
	if ep1.ID == ep2.ID {
		t.Fatal("episodes with the same subject must not collapse")
	}
	list, err := b.ListUnconsolidatedEpisodes(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Fatalf("episodes collapsed: %#v", list)
	}
}

func TestHours_ParseAndAsleep(t *testing.T) {
	h := memory.ParseHours("sleep: 23:00-07:00\nwork: 09:00-17:00\nquiet: (none)\n")
	if !h.Known() || h.SleepStart != 23*60 || h.SleepEnd != 7*60 {
		t.Fatalf("%+v", h)
	}
	if h.WorkStart != 9*60 || h.WorkEnd != 17*60 {
		t.Fatalf("work %+v", h)
	}
	loc := time.UTC
	asleep := time.Date(2026, 8, 25, 2, 0, 0, 0, loc)
	awake := time.Date(2026, 8, 25, 11, 0, 0, 0, loc)
	if !h.AsleepAt(asleep) {
		t.Fatal("02:00 should be asleep")
	}
	if h.AsleepAt(awake) {
		t.Fatal("11:00 should not be asleep (work is not DND)")
	}
	edge := time.Date(2026, 8, 25, 7, 0, 0, 0, loc)
	if h.AsleepAt(edge) {
		t.Fatal("07:00 is exclusive end")
	}
	unknown := memory.ParseHours("")
	if unknown.Known() || unknown.AsleepAt(asleep) {
		t.Fatal("unknown hours must not skip spark")
	}
	if !strings.Contains(unknown.Footer(), "unknown") {
		t.Fatalf("footer=%q", unknown.Footer())
	}
	if !strings.Contains(h.Footer(), "sleep 23:00-07:00") || !strings.Contains(h.Footer(), "work 09:00-17:00") {
		t.Fatalf("footer=%q", h.Footer())
	}
}

func TestTools_CallRoundTrip(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	tools := memory.Tools{Backend: b}
	out, err := tools.Call(ctx, memory.ToolStore, []byte(`{"kind":"fact","subject":"x","content":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty store result")
	}
	out, err = tools.Call(ctx, memory.ToolRecall, []byte(`{"query":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "no memories matched" {
		t.Fatal(out)
	}
	out, err = tools.Call(ctx, memory.ToolForget, []byte(`{"query":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty forget result")
	}
}
