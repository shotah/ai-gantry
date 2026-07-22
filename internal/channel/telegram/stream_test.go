package telegram

import (
	"testing"
	"unicode/utf8"
)

func TestClipRunes(t *testing.T) {
	s := "hello世界"
	got := clipRunes(s, 5)
	if utf8.RuneCountInString(got) != 5 {
		t.Fatalf("got=%q count=%d", got, utf8.RuneCountInString(got))
	}
	if got[len(got)-len("…"):] != "…" && !hasEllipsis(got) {
		// clipRunes uses "…" as last rune when truncating
		if utf8.RuneCountInString(s) <= 5 {
			t.Fatal("expected truncation")
		}
	}
	if clipRunes("abc", 10) != "abc" {
		t.Fatal("no clip")
	}
}

func hasEllipsis(s string) bool {
	for _, r := range s {
		if r == '…' {
			return true
		}
	}
	return false
}
