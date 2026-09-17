package channel_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestReactionSink_OnContext(t *testing.T) {
	if channel.ReactionSinkFrom(context.Background()) != nil {
		t.Fatal("bare ctx must have no sink")
	}
	ctx, sink := channel.AttachReactionSink(context.Background())
	if channel.ReactionSinkFrom(ctx) != sink || sink.Emoji() != "" {
		t.Fatal("sink not on context or not empty")
	}
	sink.Set(" 👍 ")
	if channel.ReactionSinkFrom(ctx).Emoji() != "👍" {
		t.Fatalf("emoji = %q", sink.Emoji())
	}
	var nilSink *channel.ReactionSink
	nilSink.Set("👍")
	if nilSink.Emoji() != "" {
		t.Fatal("nil sink must be inert")
	}
}

func TestInPalette(t *testing.T) {
	for _, e := range channel.Palette {
		if !channel.InPalette(e) {
			t.Fatalf("%q not in its own palette", e)
		}
	}
	if !channel.InPalette("❤") {
		t.Fatal("heart without the variation selector is still a heart")
	}
	// A reaction can say no, or cry — not only nod.
	for _, e := range []string{"👎", "😢", "🤔"} {
		if !channel.InPalette(e) {
			t.Fatalf("%q must be in the palette", e)
		}
	}
	for _, e := range []string{"💩", "✅", "😂", "", "thumbs up"} {
		if channel.InPalette(e) {
			t.Fatalf("%q must not be in the palette", e)
		}
	}
}

func TestFormatParseReaction(t *testing.T) {
	line := channel.FormatReaction([]string{"👍", "🔥"}, "Set a 6pm wake.")
	if line != "[reaction] 👍 🔥 on: Set a 6pm wake." {
		t.Fatalf("line = %q", line)
	}
	emojis, target, ok := channel.ParseReaction(line)
	if !ok || len(emojis) != 2 || emojis[0] != "👍" || emojis[1] != "🔥" || target != "Set a 6pm wake." {
		t.Fatalf("parse = %v %q %v", emojis, target, ok)
	}
	if strings.Contains(channel.FormatReaction([]string{"👍"}, ""), "on: (unknown message)") == false {
		t.Fatal("empty target must read as unknown")
	}
	long := strings.Repeat("x", 300)
	if got := channel.FormatReaction([]string{"👍"}, long); !strings.HasSuffix(got, "…") || len([]rune(got)) > 230 {
		t.Fatalf("target not clipped: %d runes", len([]rune(got)))
	}
	for _, bad := range []string{"hello", "[reaction]", "[reaction] ", "[reactions] 👍 on: x"} {
		if _, _, ok := channel.ParseReaction(bad); ok {
			t.Fatalf("%q must not parse", bad)
		}
	}
}

func TestAllPositive(t *testing.T) {
	if !channel.AllPositive([]string{"👍"}) || !channel.AllPositive([]string{"❤️", "🔥"}) {
		t.Fatal("plain acknowledgments are positive")
	}
	for _, set := range [][]string{{"👎"}, {"👍", "❓"}, {"[custom:123]"}, {}} {
		if channel.AllPositive(set) {
			t.Fatalf("%v must not be all positive", set)
		}
	}
}

func TestSettler_LatestWinsAndClearCancels(t *testing.T) {
	s := channel.NewSettler()
	var mu sync.Mutex
	var fired [][]string
	fire := func(set []string) {
		mu.Lock()
		fired = append(fired, set)
		mu.Unlock()
	}
	const quiet = 30 * time.Millisecond

	s.Schedule("a", []string{"❤️"}, quiet, fire)
	s.Schedule("a", []string{"👍"}, quiet, fire)
	s.Schedule("b", []string{"🔥"}, quiet, fire)
	s.Schedule("b", nil, quiet, fire) // cleared before it settled
	time.Sleep(4 * quiet)

	mu.Lock()
	defer mu.Unlock()
	if len(fired) != 1 || len(fired[0]) != 1 || fired[0][0] != "👍" {
		t.Fatalf("fired = %v, want one 👍", fired)
	}
	var nilSettler *channel.Settler
	nilSettler.Schedule("x", []string{"👍"}, quiet, fire)
}

func TestRecent_RingEvictsOldest(t *testing.T) {
	r := channel.NewRecent(2)
	r.Remember("r1", "one")
	r.Remember("r2", "two")
	r.Remember("r3", "three")
	if _, ok := r.Lookup("r1"); ok {
		t.Fatal("r1 should be evicted")
	}
	if got, ok := r.Lookup("r3"); !ok || got != "three" {
		t.Fatalf("r3 = %q %v", got, ok)
	}
	r.Remember("", "ignored")
	r.Remember("r4", "  ")
	if _, ok := r.Lookup("r4"); ok {
		t.Fatal("blank text must not be remembered")
	}
	var nilRecent *channel.Recent
	nilRecent.Remember("x", "y")
	if _, ok := nilRecent.Lookup("x"); ok {
		t.Fatal("nil ring must be inert")
	}
}
