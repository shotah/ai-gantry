package cron

import "strings"

// SilentToken is the reserved reply that skips the outbound chat push.
// The job still runs (tools, session history); only the human-facing message is dropped.
const SilentToken = "[silent]"

// IsSilentReply reports whether the model chose not to message the human.
// The first non-empty line must be [silent] (case-insensitive). Extra lines
// after that are ignored — models often add a reason.
func IsSilentReply(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	line := s
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		line = strings.TrimSpace(s[:i])
	}
	return strings.EqualFold(line, SilentToken)
}
