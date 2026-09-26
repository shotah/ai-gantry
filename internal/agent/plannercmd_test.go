package agent

import "testing"

func TestParsePlannerCommand(t *testing.T) {
	tests := []struct {
		in  string
		arg string
		ok  bool
	}{
		{"/planner", "", true},
		{"/planner on", "on", true},
		{"/planner off", "off", true},
		{"/planner 09:30", "09:30", true},
		{"/planner 9:30AM", "9:30am", true},
		{"/planner@bot on", "on", true},
		{"/PLANNER Off", "off", true},
		{"/spark", "", false},
		{"/engagement off", "", false},
		{"/examples", "", false},
		{"planner on", "", false},
	}
	for _, tc := range tests {
		arg, ok := parsePlannerCommand(tc.in)
		if ok != tc.ok || arg != tc.arg {
			t.Fatalf("%q: got (%q,%v) want (%q,%v)", tc.in, arg, ok, tc.arg, tc.ok)
		}
	}
}

func TestNormalizePlannerAtArg(t *testing.T) {
	got, err := normalizePlannerAtArg("9:30am")
	if err != nil || got != "09:30" {
		t.Fatalf("9:30am: %q %v", got, err)
	}
	got, err = normalizePlannerAtArg("07:10")
	if err != nil || got != "07:10" {
		t.Fatalf("07:10: %q %v", got, err)
	}
	if _, err := normalizePlannerAtArg("3-5"); err == nil {
		t.Fatal("expected error for a count")
	}
	if _, err := normalizePlannerAtArg("nope"); err == nil {
		t.Fatal("expected error")
	}
}
