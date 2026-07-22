package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitMessage_Short(t *testing.T) {
	parts := splitMessage("hello", 4096)
	if len(parts) != 1 || parts[0] != "hello" {
		t.Fatalf("%q", parts)
	}
}

func TestSplitMessage_PrefersNewline(t *testing.T) {
	text := strings.Repeat("a", 20) + "\n" + strings.Repeat("b", 20)
	parts := splitMessage(text, 25)
	if len(parts) < 2 {
		t.Fatalf("want split, got %#v", parts)
	}
	if !strings.HasSuffix(parts[0], "\n") && !strings.Contains(parts[0], "a") {
		t.Fatalf("unexpected first chunk %q", parts[0])
	}
	for _, p := range parts {
		if utf8.RuneCountInString(p) > 25 {
			t.Fatalf("chunk too long: %d", utf8.RuneCountInString(p))
		}
	}
}

func TestSplitMessage_HardCut(t *testing.T) {
	text := strings.Repeat("x", 100)
	parts := splitMessage(text, 30)
	total := 0
	for _, p := range parts {
		total += utf8.RuneCountInString(p)
		if utf8.RuneCountInString(p) > 30 {
			t.Fatalf("chunk > 30")
		}
	}
	if total != 100 {
		t.Fatalf("lost runes: %d", total)
	}
}
