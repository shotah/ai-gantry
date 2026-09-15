package agent

import (
	"strings"
	"testing"
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
