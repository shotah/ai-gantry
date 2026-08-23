package mcp

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Reason is a stable skip / tool-error code a model and a UI can switch on.
// Do not rename these strings: gantree and slog consumers match them.
type Reason string

// Stable reason codes. Do not rename: gantree and slog consumers match them.
const (
	ReasonNoBinary Reason = "no_binary"
	ReasonNoKey    Reason = "no_key"
	ReasonNoOAuth  Reason = "no_oauth"
	ReasonConnect  Reason = "connect"
)

// ClassifiedError tags a boot or dial failure with a Reason without changing
// the underlying message (so skip notes stay human).
type ClassifiedError struct {
	Reason Reason
	Err    error
}

func (e *ClassifiedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Reason)
}

func (e *ClassifiedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// UnavailableError is returned when the model calls a prefix that was
// boot-skipped. The code is in the string so both the Completer and a UI
// can tell no_binary from no_key from no_oauth.
type UnavailableError struct {
	Server string
	Reason Reason
	Note   string
}

func (e *UnavailableError) Error() string {
	if e == nil {
		return "tool error [connect]: server is skipped"
	}
	code := e.Reason
	if code == "" {
		code = ReasonConnect
	}
	note := strings.TrimSpace(e.Note)
	if note != "" {
		return fmt.Sprintf("tool error [%s]: %s is skipped (%s) — do not invent %s__* names", code, e.Server, note, e.Server)
	}
	return fmt.Sprintf("tool error [%s]: %s is skipped — do not invent %s__* names", code, e.Server, e.Server)
}

// ReasonOf returns the classified reason for a boot or call error.
func ReasonOf(err error) Reason {
	if err == nil {
		return ""
	}
	var ce *ClassifiedError
	if errors.As(err, &ce) && ce.Reason != "" {
		return ce.Reason
	}
	var u *UnavailableError
	if errors.As(err, &u) && u.Reason != "" {
		return u.Reason
	}
	if errors.Is(err, exec.ErrNotFound) {
		return ReasonNoBinary
	}
	return ClassifyReason(err.Error())
}

// ClassifyReason maps a free-text error to a stable code. Cheap: no I/O.
func ClassifyReason(msg string) Reason {
	s := strings.ToLower(strings.TrimSpace(msg))
	if s == "" {
		return ReasonConnect
	}
	switch {
	case strings.Contains(s, "executable file not found"),
		strings.Contains(s, "not found in $path"),
		strings.Contains(s, "not found in %path%"),
		looksLikeMissingBinary(s):
		return ReasonNoBinary
	case strings.Contains(s, "api key"),
		strings.Contains(s, "apikey"),
		strings.Contains(s, "missing env"),
		strings.Contains(s, "environment variable"),
		strings.Contains(s, "required env"),
		strings.Contains(s, "no key"),
		strings.Contains(s, "api_key"):
		return ReasonNoKey
	case strings.Contains(s, "oauth"),
		strings.Contains(s, "unauthenticated"),
		strings.Contains(s, "not authenticated"),
		strings.Contains(s, "login required"),
		strings.Contains(s, "authorize"),
		strings.Contains(s, "invalid_grant"),
		strings.Contains(s, "refresh token"),
		strings.Contains(s, "no token"),
		strings.Contains(s, "token expired"),
		strings.Contains(s, "401"):
		return ReasonNoOAuth
	default:
		return ReasonConnect
	}
}

func looksLikeMissingBinary(s string) bool {
	if strings.Contains(s, "no such file") {
		return true
	}
	// exec.LookPath / exec.Command: `exec: "foo": executable file not found`
	return strings.HasPrefix(s, "exec:") && strings.Contains(s, "not found")
}

func emptyEnvKeys(env []string) []string {
	var missing []string
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if strings.TrimSpace(os.Expand(v, os.Getenv)) == "" {
			missing = append(missing, k)
		}
	}
	return missing
}

func classifiedToolError(text string) error {
	text = strings.TrimSpace(text)
	r := ClassifyReason(text)
	if r == ReasonNoKey || r == ReasonNoOAuth || r == ReasonNoBinary {
		return fmt.Errorf("tool error [%s]: %s", r, text)
	}
	return fmt.Errorf("tool error: %s", text)
}
