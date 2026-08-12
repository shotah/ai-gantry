package agent

import (
	"testing"
)

func TestParseExamplesCommand(t *testing.T) {
	tests := []struct {
		in  string
		arg string
		ok  bool
	}{
		{"/examples", "", true},
		{"/examples on", "on", true},
		{"/examples off", "off", true},
		{"/examples true", "true", true},
		{"/examples false", "false", true},
		{"/examples@bot on", "on", true},
		{"/EXAMPLES Off", "off", true},
		{"/tools", "", false},
		{"examples on", "", false},
	}
	for _, tc := range tests {
		arg, ok := parseExamplesCommand(tc.in)
		if ok != tc.ok || arg != tc.arg {
			t.Fatalf("%q: got (%q,%v) want (%q,%v)", tc.in, arg, ok, tc.arg, tc.ok)
		}
	}
}
