package agent

import "testing"

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
	if got != harnessClockNote+"\n[current time] NOW: x" {
		t.Fatalf("got %q", got)
	}
	if formatHarnessClock("  ") != "" {
		t.Fatal("empty clock")
	}
}
