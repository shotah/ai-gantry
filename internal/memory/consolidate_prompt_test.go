package memory

import (
	"strings"
	"testing"
	"time"
)

func TestBuildConsolidatePrompt_IncludesRecorded(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("America/Los_Angeles", -7*3600)
	got := buildConsolidatePrompt([]Entry{{
		ID:        3,
		Subject:   "today",
		Content:   "skipped gym",
		CreatedAt: time.Date(2026, time.August, 10, 21, 0, 0, 0, loc),
	}}, loc)
	if !strings.Contains(got, `recorded=Mon 2026-08-10`) {
		t.Fatalf("got %q, want local recorded date", got)
	}
	if strings.Contains(got, "2026-08-11") {
		t.Fatalf("UTC calendar day leaked: %q", got)
	}
}
