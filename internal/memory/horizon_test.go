package memory

import (
	"strings"
	"testing"
)

func TestFormatAimsAndLoops(t *testing.T) {
	if FormatAims(nil) != "" || FormatLoops(nil) != "" {
		t.Fatal("empty")
	}
	aims := FormatAims([]Entry{
		{Subject: "aim/training", Content: "3x gym this month"},
		{Subject: "aim/spanish", Content: "B1 by summer"},
	})
	if aims != "[aims] training: 3x gym this month · spanish: B1 by summer" {
		t.Fatalf("aims %q", aims)
	}
	loops := FormatLoops([]Entry{
		{Subject: "waiting/dentist", Content: "book cleaning"},
		{Subject: "follow/visa", Content: "packet in"},
	})
	if loops != "[loops] waiting/dentist: book cleaning · follow/visa: packet in" {
		t.Fatalf("loops %q", loops)
	}
}

func TestFormatAims_ClipsAndCaps(t *testing.T) {
	got := FormatAims([]Entry{{Subject: "aim/x", Content: strings.Repeat("x", 90)}})
	if !strings.HasPrefix(got, "[aims] x: ") || !strings.HasSuffix(got, "…") {
		t.Fatalf("clip %q", got)
	}
	many := make([]Entry, 8)
	for i := range many {
		many[i] = Entry{Subject: "aim/" + string(rune('a'+i)), Content: "n"}
	}
	got = FormatAims(many)
	if strings.Count(got, " · ") != harnessHorizonMax-1 {
		t.Fatalf("cap %q", got)
	}
}
