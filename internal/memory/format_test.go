package memory_test

import (
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/memory"
)

func TestFormatHydration_StampsLocalDate(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("America/Los_Angeles", -7*3600)
	created := time.Date(2026, time.August, 10, 21, 0, 0, 0, loc) // Mon evening PT
	got := memory.FormatHydration([]memory.Entry{{
		Kind:      memory.KindEpisode,
		Subject:   "chris",
		Content:   "skipped gym while traveling",
		CreatedAt: created.UTC(),
	}}, loc)
	if !strings.Contains(got, "(episode, Mon 2026-08-10)") {
		t.Fatalf("got %q, want local weekday+date (not UTC Tuesday)", got)
	}
	if strings.Contains(got, "2026-08-11") {
		t.Fatalf("UTC calendar day leaked: %q", got)
	}
}

func TestFormatHydration_ZeroCreatedOmitsDate(t *testing.T) {
	t.Parallel()
	got := memory.FormatHydration([]memory.Entry{{
		Kind:    memory.KindFact,
		Subject: "climbing",
		Content: "trains Tue/Thu",
	}}, time.UTC)
	if got != "[memory]\n- (fact) climbing: trains Tue/Thu" {
		t.Fatalf("got %q", got)
	}
}
