package agent

import "testing"

func TestParseEnableHoldCommand(t *testing.T) {
	cmd, prefix, ok := parseEnableHoldCommand("/short google__calendar")
	if !ok || cmd != "/short" || prefix != "google__calendar" {
		t.Fatalf("got %q %q %v", cmd, prefix, ok)
	}
	cmd, prefix, ok = parseEnableHoldCommand("/brief flights")
	if !ok || cmd != "/brief" || prefix != "flights" {
		t.Fatalf("brief: %q %q %v", cmd, prefix, ok)
	}
	if _, _, ok := parseEnableHoldCommand("/short"); ok {
		t.Fatal("bare /short should not parse")
	}
	if _, _, ok := parseEnableHoldCommand("/tools"); ok {
		t.Fatal("/tools is not a hold command")
	}
}
