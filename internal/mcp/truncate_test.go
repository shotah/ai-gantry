package mcp_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/shotah/ai-gantry/internal/mcp"
)

func TestTruncate(t *testing.T) {
	if got := mcp.Truncate("hi", 10); got != "hi" {
		t.Fatal(got)
	}
	long := strings.Repeat("x", 100)
	got := mcp.Truncate(long, 20)
	if utf8.RuneCountInString(got) > 20 {
		t.Fatalf("len=%d", utf8.RuneCountInString(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("%q", got)
	}
}

func TestPrefixedName(t *testing.T) {
	got, err := mcp.PrefixedName("google-workspace", "gmail.search")
	if err != nil {
		t.Fatal(err)
	}
	if got != "google-workspace__gmail_search" {
		t.Fatalf("%q", got)
	}
}
