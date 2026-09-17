package cron_test

import (
	"testing"

	"github.com/shotah/ai-gantry/internal/cron"
)

func TestReactToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, emoji, stripped string
	}{
		{"[react 👍]", "👍", ""},
		{"[REACT ❤️]", "❤️", ""},
		{"Set a 6pm wake to ask how it went.\n[react 👍]", "👍", "Set a 6pm wake to ask how it went."},
		{"Set a 6pm wake. [react 👍]", "👍", "Set a 6pm wake."},
		{"[react 👍]\n[wait]", "👍", ""},
		{"[react 👍] [wait]", "👍", ""},
		{"[react 🔥]\n[react 👍]", "🔥", ""},
		// Not tokens: no body, unclosed, mid-sentence.
		{"[react ]", "", "[react ]"},
		{"[react 👍", "", "[react 👍"},
		{"I will [react 👍] later", "", "I will [react 👍] later"},
		{"hello", "", "hello"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := cron.ReactEmoji(tc.in); got != tc.emoji {
			t.Fatalf("ReactEmoji(%q)=%q want %q", tc.in, got, tc.emoji)
		}
		if got := cron.StripWaitTokens(tc.in); got != tc.stripped {
			t.Fatalf("StripWaitTokens(%q)=%q want %q", tc.in, got, tc.stripped)
		}
	}
	if cron.IsSilentReply("[react 👍]") {
		t.Fatal("a reaction is not [silent]")
	}
}

func TestStripWaitTokensLive_React(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"Nice.\n[", "Nice."},
		{"Nice.\n[re", "Nice."},
		{"Nice.\n[react ", "Nice."},
		{"Nice.\n[react 👍", "Nice."},
		{"Nice.\n[react 👍]", "Nice."},
		{"Nice. [rea", "Nice."},
		{"Nice. [react 👍", "Nice."},
		{"Nice. [react 👍]", "Nice."},
		{"[react 👍", ""},
		{"[reach out]", "[reach out]"},
	}
	for _, tc := range cases {
		if got := cron.StripWaitTokensLive(tc.in); got != tc.want {
			t.Fatalf("StripWaitTokensLive(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
