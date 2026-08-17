package agent

import "testing"

func TestParseEnableHoldCommand(t *testing.T) {
	cmd, prefix, ok := parseEnableHoldCommand("/long google__calendar")
	if !ok || cmd != "/long" || prefix != "google__calendar" {
		t.Fatalf("got %q %q %v", cmd, prefix, ok)
	}
	if _, _, ok := parseEnableHoldCommand("/long"); ok {
		t.Fatal("bare /long should not parse")
	}
	if _, _, ok := parseEnableHoldCommand("/tools"); ok {
		t.Fatal("/tools is not a hold command")
	}
}
