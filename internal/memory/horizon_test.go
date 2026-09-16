package memory

import (
	"strings"
	"testing"
	"time"
)

func TestFormatAimsAndLoops(t *testing.T) {
	if FormatAims(nil, time.Time{}) != "" || FormatLoops(nil, nil, time.Time{}) != "" {
		t.Fatal("empty")
	}
	aims := FormatAims([]Entry{
		{Subject: "aim/training", Content: "3x gym this month"},
		{Subject: "aim/spanish", Content: "B1 by summer"},
	}, time.Time{})
	if aims != "[aims] training: 3x gym this month · spanish: B1 by summer" {
		t.Fatalf("aims %q", aims)
	}
	loops := FormatLoops(
		[]Entry{{Subject: "waiting/dentist", Content: "book cleaning"}},
		[]Entry{{Subject: "follow/visa", Content: "packet in"}},
		time.Time{},
	)
	if loops != "[loops] waiting/dentist: book cleaning · follow/visa: packet in" {
		t.Fatalf("loops %q", loops)
	}
}

// The bootstrap marker renders the ask date in the operator's zone so the
// "once per day" rule is answered by the stamp, not by a memory_recall.
func TestFormatAimsEmpty(t *testing.T) {
	if FormatAimsEmpty(nil, time.Time{}) != "" {
		t.Fatal("never asked must leave the line absent")
	}
	la, _ := time.LoadLocation("America/Los_Angeles")
	// 04:30Z on the 16th is still the 15th in LA.
	asked := Entry{Subject: SubjectAimBootstrap, UpdatedAt: time.Date(2026, 9, 16, 4, 30, 0, 0, time.UTC)}
	now := time.Date(2026, 9, 16, 6, 0, 0, 0, la)
	if got := FormatAimsEmpty(&asked, now); got != "[aims] none (asked 2026-09-15)" {
		t.Fatalf("got %q", got)
	}
	if got := FormatAimsEmpty(&Entry{Subject: SubjectAimBootstrap}, now); got != "[aims] none (asked)" {
		t.Fatalf("no timestamp: %q", got)
	}
}

func TestFormatAims_ClipsAndCountsOverflow(t *testing.T) {
	got := FormatAims([]Entry{{Subject: "aim/x", Content: strings.Repeat("x", 90)}}, time.Time{})
	if !strings.HasPrefix(got, "[aims] x: ") || !strings.HasSuffix(got, "…") {
		t.Fatalf("clip %q", got)
	}
	many := make([]Entry, 8)
	for i := range many {
		many[i] = Entry{Subject: "aim/" + string(rune('a'+i)), Content: "n"}
	}
	got = FormatAims(many, time.Time{})
	if strings.Count(got, " · ") != harnessHorizonMax-1 {
		t.Fatalf("cap %q", got)
	}
	if !strings.HasSuffix(got, " (+3 more — memory_recall aim/)") {
		t.Fatalf("overflow count missing %q", got)
	}
}

func TestFormatLoops_FollowSurvivesFiveWaits(t *testing.T) {
	waiting := make([]Entry, 5)
	for i := range waiting {
		waiting[i] = Entry{Subject: "waiting/w" + string(rune('0'+i)), Content: "n"}
	}
	follow := []Entry{{Subject: "follow/visa", Content: "packet in"}}
	got := FormatLoops(waiting, follow, time.Time{})
	if !strings.Contains(got, "follow/visa: packet in") {
		t.Fatalf("follow starved %q", got)
	}
	if !strings.HasPrefix(got, "[loops] waiting/w0: n · follow/visa: packet in · waiting/w1") {
		t.Fatalf("interleave order %q", got)
	}
	if !strings.HasSuffix(got, " (+1 more — memory_recall waiting/ follow/)") {
		t.Fatalf("overflow %q", got)
	}
}

func TestHorizonAge_StampsDaysAndStaleCue(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	fresh := Entry{Subject: "aim/a", Content: "n", UpdatedAt: now.Add(-3 * time.Hour)}
	old := Entry{Subject: "aim/b", Content: "n", UpdatedAt: now.Add(-12 * 24 * time.Hour)}
	got := FormatAims([]Entry{fresh, old}, now)
	if got != "[aims] a: n · b: n (12d ago)" {
		t.Fatalf("aims age %q", got)
	}
	stale := Entry{Subject: "waiting/dentist", Content: "book", CreatedAt: now.Add(-35 * 24 * time.Hour)}
	got = FormatLoops([]Entry{stale}, nil, now)
	if got != "[loops] waiting/dentist: book (35d ago — resolve or memory_forget)" {
		t.Fatalf("stale cue %q", got)
	}
	live := Entry{Subject: "follow/visa", Content: "packet", UpdatedAt: now.Add(-2 * 24 * time.Hour)}
	if got = FormatLoops(nil, []Entry{live}, now); got != "[loops] follow/visa: packet (2d ago)" {
		t.Fatalf("live loop %q", got)
	}
}
