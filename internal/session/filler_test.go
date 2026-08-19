package session_test

import (
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/session"
)

func TestStripFillerWords_KeepsToneAndNegation(t *testing.T) {
	got := session.StripFillerWords("the gull had a mortgage")
	if got != "gull had mortgage" {
		t.Fatalf("got %q", got)
	}
	// "that" is not in the list — joke setup stays.
	got = session.StripFillerWords("that gull had a mortgage")
	if got != "that gull had mortgage" {
		t.Fatalf("that: %q", got)
	}
	got = session.StripFillerWords("do not call the tool")
	if !strings.Contains(got, "not") || strings.Contains(got, " the ") {
		t.Fatalf("negation: %q", got)
	}
	got = session.StripFillerWords("just wait oh like")
	if got != "just wait oh like" {
		t.Fatalf("discourse particles stripped: %q", got)
	}
	got = session.StripFillerWords("the gull had a mortgage")
	if !strings.Contains(got, "had") {
		t.Fatalf("stripped main-verb had: %q", got)
	}
}

func TestStripFillerWords_LongHedgesAndPhrases(t *testing.T) {
	got := session.StripFillerWords("actually check the calendar you know")
	if strings.Contains(got, "actually") || strings.Contains(got, "you know") {
		t.Fatalf("hedge left: %q", got)
	}
	if !strings.Contains(got, "check") || !strings.Contains(got, "calendar") {
		t.Fatalf("content lost: %q", got)
	}
	got = session.StripFillerWords("please thanks")
	if got != "please thanks" {
		t.Fatalf("politeness stripped: %q", got)
	}
	got = session.StripFillerWords("it was kind of late")
	if strings.Contains(got, "kind of") {
		t.Fatalf("phrase left: %q", got)
	}
	got = session.StripFillerWords(`he said "actually the gull" basically`)
	if !strings.Contains(got, `"actually the gull"`) {
		t.Fatalf("quoted hedge stripped: %q", got)
	}
	if strings.Contains(got, "basically") {
		t.Fatalf("unquoted hedge left: %q", got)
	}
	got = session.StripFillerWords("really literally maybe")
	if got != "really literally maybe" {
		t.Fatalf("intensifiers/epistemics stripped: %q", got)
	}
}

func TestStripFillerWords_PreservesQuotes(t *testing.T) {
	in := `he said "the gull had a mortgage" to the room`
	got := session.StripFillerWords(in)
	if !strings.Contains(got, `"the gull had a mortgage"`) {
		t.Fatalf("quote damaged: %q", got)
	}
	if strings.Contains(got, " to the ") {
		t.Fatalf("unquoted fillers left: %q", got)
	}
}

func TestStripFillerHistory_LeavesTailVerbatim(t *testing.T) {
	n := session.KeepRecentUnstripped + 5
	msgs := make([]session.Message, n)
	for i := range msgs {
		msgs[i] = session.Message{Role: session.RoleUser, Content: "the calendar on Tuesday"}
	}
	msgs[n-1].Content = "the last line stays the the the"
	out := session.StripFillerHistory(msgs)
	if len(out) != n {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].Content == "the calendar on Tuesday" {
		t.Fatalf("head not stripped: %q", out[0].Content)
	}
	if !strings.Contains(out[0].Content, "calendar") || strings.Contains(out[0].Content, " the ") {
		t.Fatalf("head strip: %q", out[0].Content)
	}
	if out[n-1].Content != "the last line stays the the the" {
		t.Fatalf("tail mutated: %q", out[n-1].Content)
	}
	// Original slice must not change (prompt-only).
	if msgs[0].Content != "the calendar on Tuesday" {
		t.Fatal("stripped in place — SQLite path would be corrupted")
	}
}

func TestStripFillerHistory_SkipsAssistant(t *testing.T) {
	n := session.KeepRecentUnstripped + 4
	msgs := make([]session.Message, n)
	for i := range msgs {
		role := session.RoleUser
		content := "the calendar on Tuesday"
		if i%2 == 1 {
			role = session.RoleAssistant
			content = "the day is clear"
		}
		msgs[i] = session.Message{Role: role, Content: content}
	}
	out := session.StripFillerHistory(msgs)
	if out[0].Content == "the calendar on Tuesday" {
		t.Fatalf("old user not stripped: %q", out[0].Content)
	}
	if out[1].Content != "the day is clear" {
		t.Fatalf("assistant stripped: %q", out[1].Content)
	}
}

func TestStripFillerHistory_ShortSessionUntouched(t *testing.T) {
	msgs := []session.Message{{Content: "the short one"}}
	out := session.StripFillerHistory(msgs)
	if out[0].Content != "the short one" {
		t.Fatalf("short session stripped: %q", out[0].Content)
	}
}
