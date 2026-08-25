package agent

import (
	"testing"
)

func TestParseSparkCommand(t *testing.T) {
	tests := []struct {
		in  string
		arg string
		ok  bool
	}{
		{"/spark", "", true},
		{"/spark on", "on", true},
		{"/spark off", "off", true},
		{"/spark 4", "4", true},
		{"/spark 3-5", "3-5", true},
		{"/spark@bot 2", "2", true},
		{"/SPARK Off", "off", true},
		{"/engagement", "", true},
		{"/engagement off", "off", true},
		{"/engagement 3-5", "3-5", true},
		{"/engagement@bot on", "on", true},
		{"/examples", "", false},
		{"spark on", "", false},
	}
	for _, tc := range tests {
		arg, ok := parseSparkCommand(tc.in)
		if ok != tc.ok || arg != tc.arg {
			t.Fatalf("%q: got (%q,%v) want (%q,%v)", tc.in, arg, ok, tc.arg, tc.ok)
		}
	}
}

func TestNormalizeSparkQtyArg(t *testing.T) {
	got, err := normalizeSparkQtyArg("4")
	if err != nil || got != "4" {
		t.Fatalf("4: %q %v", got, err)
	}
	got, err = normalizeSparkQtyArg("3-5")
	if err != nil || got != "3-5" {
		t.Fatalf("3-5: %q %v", got, err)
	}
	if _, err := normalizeSparkQtyArg("0"); err == nil {
		t.Fatal("expected error for 0")
	}
	if _, err := normalizeSparkQtyArg("nope"); err == nil {
		t.Fatal("expected error")
	}
}
