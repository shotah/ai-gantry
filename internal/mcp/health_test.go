package mcp_test

import (
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/mcp"
)

func TestFormatServerHealth_OmitsSkippedAndListsIdle(t *testing.T) {
	now := time.Date(2026, 8, 16, 20, 0, 0, 0, time.UTC)
	got := mcp.FormatServerHealth([]mcp.ServerStatus{
		{Name: "cast", State: mcp.ServerSkipped, Note: "no such file"},
		{Name: "garmin", State: mcp.ServerError, At: now.Add(-4 * time.Minute), Tool: "garmin__sleep_get", Note: "401 unauthorized"},
		{Name: "google", State: mcp.ServerOK, At: now.Add(-2 * time.Minute), Tool: "google__calendar_events_list"},
		{Name: "math", State: mcp.ServerIdle},
	}, now)
	if strings.Contains(got, "cast") || strings.Contains(got, "skipped") {
		t.Fatalf("prompt listed a skipped server: %q", got)
	}
	for _, want := range []string{
		"[tools]",
		"garmin  error  4m ago  sleep_get: 401 unauthorized",
		"google  ok     2m ago  calendar_events_list",
		"math    idle",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatServerHealthLine_Skipped(t *testing.T) {
	got := mcp.FormatServerHealthLine(mcp.ServerStatus{
		Name:   "cast",
		State:  mcp.ServerSkipped,
		Reason: mcp.ReasonNoBinary,
		Note:   "executable not found",
	}, time.Now())
	if got != "skipped  no_binary  executable not found" {
		t.Fatalf("%q", got)
	}
	legacy := mcp.FormatServerHealthLine(mcp.ServerStatus{
		Name:  "cast",
		State: mcp.ServerSkipped,
		Note:  "executable not found",
	}, time.Now())
	if legacy != "skipped  executable not found" {
		t.Fatalf("%q", legacy)
	}
}
