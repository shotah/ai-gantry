package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

func TestStripHarnessContext_TrailingClock(t *testing.T) {
	in := "what's near me\n\n[location ±8m] 47.600000, -122.300000\n[current time] NOW: Saturday\n[hours] unknown"
	if got := stripHarnessContext(in); got != "what's near me" {
		t.Fatalf("got %q", got)
	}
}

func TestStripHarnessContext_LeadingHarness(t *testing.T) {
	in := "[harness] Not user text — location, clock, and hours for this turn.\n[current time] NOW: x\n\nhello"
	if got := stripHarnessContext(in); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestStripHarnessContext_GluedFooter(t *testing.T) {
	in := "hello\n[current time] NOW: x\nalready today: y"
	if got := stripHarnessContext(in); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestStripHarnessContext_KeepsMention(t *testing.T) {
	in := "what does [hours] mean in the footer"
	if got := stripHarnessContext(in); got != in {
		t.Fatalf("got %q", got)
	}
}

func TestStripHarnessContext_MemoryBlock(t *testing.T) {
	in := "[memory]\n- (fact) x: y\n\nreal ask"
	if got := stripHarnessContext(in); got != "real ask" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatHarnessClock(t *testing.T) {
	got := formatHarnessClock("[current time] NOW: x")
	if got != "[harness] Not user text — clock for this turn.\n[current time] NOW: x" {
		t.Fatalf("got %q", got)
	}
	if formatHarnessClock("  ") != "" {
		t.Fatal("empty clock")
	}
}

func TestHarnessNote_NamesOnlyPresentTags(t *testing.T) {
	cases := map[string]string{
		"[location ±8m] x\n[current time] y":                                    "location and clock",
		"[location] x\n[current time] y\n[hours] z\n[aims] a\n[loops] l":        "location, clock, hours, and horizon",
		"[current time] y\n[wakes] w\n[surface] android_auto\n[last contact] c": "clock, wakes, surface, and last contact",
	}
	for clock, want := range cases {
		if got := harnessNote(clock); got != harnessNotePrefix+want+" for this turn." {
			t.Fatalf("%q → %q", clock, got)
		}
	}
}

func TestStripHarnessContext_NewStamps(t *testing.T) {
	in := "hi\n[wakes] 5:00 PM gym\n[surface] android_auto\n[last contact] 3h ago"
	if got := stripHarnessContext(in); got != "hi" {
		t.Fatalf("got %q", got)
	}
}

func TestSparkNotes_ReadHarnessNotRecall(t *testing.T) {
	if !strings.Contains(sparkToolFirstNote, "[hours], [aims], [loops], and [wakes] are already in [harness]") {
		t.Fatalf("sparkToolFirstNote must point at the stamps: %q", sparkToolFirstNote)
	}
	for _, s := range []string{"memory_recall for aim/, pref/hours", "cron_list, then live", "Empty aim board"} {
		if strings.Contains(sparkToolFirstNote, s) {
			t.Fatalf("sparkToolFirstNote still says %q", s)
		}
	}
	// The room nudge is one clause, conditional on the stamp, so a Telegram
	// spark does not chase a phone it cannot dress and nobody is handed a recipe.
	if !strings.Contains(sparkToolFirstNote, "If [room] is stamped and stale, redress it") {
		t.Fatalf("sparkToolFirstNote must redress [room] only when stamped: %q", sparkToolFirstNote)
	}
	if strings.Contains(sparkToolFirstNote, "theme_list") || strings.Contains(sparkToolFirstNote, "theme_update") {
		t.Fatalf("sparkToolFirstNote must not carry room tool recipes: %q", sparkToolFirstNote)
	}
}

type fixedRoom struct{ r channel.Room }

func (f fixedRoom) Room() channel.Room { return f.r }

// catalogTools is a Tools with a fixed catalog; roomStamp only reads Tools().
type catalogTools struct{ defs []provider.ToolDef }

func (c catalogTools) Tools() []provider.ToolDef { return c.defs }

func (c catalogTools) ToolCount() int { return len(c.defs) }

func (catalogTools) Call(context.Context, string, json.RawMessage) (string, error) {
	return "", nil
}

func roomAgent(room RoomSource, catalog []provider.ToolDef) *Agent {
	return &Agent{room: room, tools: catalogTools{defs: catalog}}
}

func defs(names ...string) []provider.ToolDef {
	out := make([]provider.ToolDef, 0, len(names))
	for _, n := range names {
		out = append(out, provider.ToolDef{Name: n})
	}
	return out
}

// [room] needs both prerequisites: the pendant mouth and the pendant MCP.
func TestRoomStamp_NeedsMouthAndMCP(t *testing.T) {
	now := time.Now()
	pendant := defs("pendant__theme_list", "pendant__theme_update", "memory_store")
	if got := (&Agent{tools: catalogTools{defs: pendant}}).roomStamp(now, pendant, false); got != "" {
		t.Fatalf("no RoomSource (Telegram) must not stamp: %q", got)
	}
	noPendant := defs("google__calendar_list_events", "memory_store")
	if got := roomAgent(fixedRoom{}, noPendant).roomStamp(now, noPendant, false); got != "" {
		t.Fatalf("pendant MCP not mounted must not stamp: %q", got)
	}
	if got := roomAgent(fixedRoom{}, pendant).roomStamp(now, nil, true); got != "" {
		t.Fatalf("tools off this turn must not stamp: %q", got)
	}
}

func TestRoomStamp_StateAgesAndNudge(t *testing.T) {
	now := time.Date(2026, 9, 14, 18, 0, 0, 0, time.UTC)
	catalog := defs("pendant__theme_list", "pendant__theme_update", "pendant__avatar_update", "image__photo_generate")
	room := fixedRoom{channel.Room{
		Theme: "ember", ThemeAt: now.Add(-3 * time.Hour),
		Backdrop: true, BackdropAt: now.Add(-3 * time.Hour),
		FaceAt: now.Add(-2 * 24 * time.Hour),
	}}
	got := roomAgent(room, catalog).roomStamp(now, catalog, false)
	want := "[room] theme ember (set 3h ago) · wallpaper set 3h ago · face changed 2d ago — yours; redress when the hour or your mood moves on"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	// State plus one clause. The tool descriptions carry the recipe; a
	// how-to here is what overloads a small model.
	for _, recipe := range []string{"theme_list", "theme_update", "photo_generate", "avatar_update", "→"} {
		if strings.Contains(got, recipe) {
			t.Fatalf("[room] must not carry tool recipes (%q): %q", recipe, got)
		}
	}
}

// Fresh boot: the mailbox does not flush theme to the crane, so say so
// rather than invent a card. Cleared pieces read as cleared.
func TestRoomStamp_UnknownAndCleared(t *testing.T) {
	now := time.Date(2026, 9, 14, 18, 0, 0, 0, time.UTC)
	catalog := defs("pendant__theme_update")
	got := roomAgent(fixedRoom{}, catalog).roomStamp(now, catalog, false)
	if !strings.HasPrefix(got, "[room] theme not seen since boot (theme_list shows it) — ") {
		t.Fatalf("unknown: %q", got)
	}
	cleared := fixedRoom{channel.Room{ThemeAt: now.Add(-10 * time.Minute), BackdropAt: now.Add(-10 * time.Minute)}}
	got = roomAgent(cleared, catalog).roomStamp(now, catalog, false)
	if !strings.HasPrefix(got, "[room] theme cleared 10m ago · no wallpaper — ") {
		t.Fatalf("cleared: %q", got)
	}
}

// Dynamic tools: pendant in the catalog but off for this chat → the line
// points at mcp_enable instead of nudging a call that would be blocked.
func TestRoomStamp_PrefixOffSaysEnable(t *testing.T) {
	now := time.Now()
	catalog := defs("pendant__theme_update", "memory_store")
	published := defs("memory_store")
	got := roomAgent(fixedRoom{}, catalog).roomStamp(now, published, false)
	if !strings.HasSuffix(got, " — pendant is off this chat") {
		t.Fatalf("off: %q", got)
	}
	if strings.Contains(got, "redress") {
		t.Fatalf("must not nudge a blocked call: %q", got)
	}
}

func TestHarnessNote_Room(t *testing.T) {
	got := harnessNote("[current time] y\n[surface] browser\n[room] theme ember\n[last contact] c")
	if got != harnessNotePrefix+"clock, surface, room, and last contact for this turn." {
		t.Fatalf("got %q", got)
	}
	if stripHarnessContext("hi\n[room] theme ember (set 3h ago)") != "hi" {
		t.Fatal("[room] must strip like the other stamps")
	}
}

func TestSurfaceStamp(t *testing.T) {
	if surfaceStamp("") != "" {
		t.Fatal("empty surface")
	}
	if got := surfaceStamp("browser"); got != "[surface] browser" {
		t.Fatalf("browser %q", got)
	}
	if got := surfaceStamp("carplay"); got != "[surface] carplay — driving: one short spoken sentence, no markdown or lists" {
		t.Fatalf("carplay %q", got)
	}
}
