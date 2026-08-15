package cron_test

import (
	"testing"

	"github.com/shotah/ai-gantry/internal/cron"
)

func TestIsSilentReply(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"[silent]", true},
		{"  [SILENT]  ", true},
		{"[Silent]\nall-clear, garmin looks fine", true},
		{"[silent]\n\n— tools: garmin__sleep_get", true},
		{"hello [silent]", false},
		{"I'll stay [silent] this time", false},
		{"Time to submit your timecard.", false},
	}
	for _, tc := range cases {
		if got := cron.IsSilentReply(tc.in); got != tc.want {
			t.Fatalf("IsSilentReply(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
