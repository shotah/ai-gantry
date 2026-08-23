package mcp

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestClassifyReason(t *testing.T) {
	cases := []struct {
		in   string
		want Reason
	}{
		{"executable file not found in $PATH", ReasonNoBinary},
		{`exec: "google-mcp": executable file not found in $PATH`, ReasonNoBinary},
		{"open /usr/bin/foo: no such file or directory", ReasonNoBinary},
		{"missing env GOOGLE_MAPS_API_KEY", ReasonNoKey},
		{"GOOGLE_MAPS_API_KEY is not set", ReasonNoKey},
		{"need an API key", ReasonNoKey},
		{"oauth token missing", ReasonNoOAuth},
		{"401 unauthorized", ReasonNoOAuth},
		{"not authenticated", ReasonNoOAuth},
		{"cannot spawn", ReasonConnect},
		{"", ReasonConnect},
	}
	for _, c := range cases {
		if got := ClassifyReason(c.in); got != c.want {
			t.Errorf("ClassifyReason(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReasonOf_Unwraps(t *testing.T) {
	err := &ClassifiedError{Reason: ReasonNoBinary, Err: exec.ErrNotFound}
	if ReasonOf(err) != ReasonNoBinary {
		t.Fatalf("%q", ReasonOf(err))
	}
	wrapped := fmt.Errorf("dial: %w", err)
	if ReasonOf(wrapped) != ReasonNoBinary {
		t.Fatalf("wrapped %q", ReasonOf(wrapped))
	}
	if ReasonOf(exec.ErrNotFound) != ReasonNoBinary {
		t.Fatal("ErrNotFound")
	}
	if ReasonOf(errors.New("cannot spawn")) != ReasonConnect {
		t.Fatal("generic")
	}
}

func TestUnavailableError_IncludesCode(t *testing.T) {
	err := &UnavailableError{Server: "google", Reason: ReasonNoOAuth, Note: "no token file"}
	s := err.Error()
	for _, want := range []string{"tool error [no_oauth]", "google is skipped", "do not invent google__*"} {
		if !strings.Contains(s, want) {
			t.Fatalf("%q missing %q", s, want)
		}
	}
}

func TestClassifiedToolError(t *testing.T) {
	got := classifiedToolError("401 unauthorized")
	if !strings.Contains(got.Error(), "tool error [no_oauth]:") {
		t.Fatalf("%v", got)
	}
	got = classifiedToolError("boom")
	if got.Error() != "tool error: boom" {
		t.Fatalf("%v", got)
	}
}

func TestEmptyEnvKeys(t *testing.T) {
	t.Setenv("PRESENT", "yes")
	t.Setenv("EMPTY", "")
	got := emptyEnvKeys([]string{"PRESENT=${PRESENT}", "EMPTY=${EMPTY}", "LITERAL=x", "NOEQ"})
	if len(got) != 1 || got[0] != "EMPTY" {
		t.Fatalf("%v", got)
	}
}
