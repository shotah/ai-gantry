package cron

import "strings"

// ReactPrefix opens the reaction reply token: `[react 👍]` on its own line
// (or trailing) puts the emoji on the human's message instead of, or beside,
// text. A reply token like [wait] and [silent], not a tool. Stripped before
// the human sees it; stored in history so the model sees it reacted.
const ReactPrefix = "[react "

// ReactEmoji returns the emoji inside the first [react …] token, "" when the
// reply has none. Whitespace inside the brackets is trimmed; the palette
// check is the caller's.
func ReactEmoji(s string) string {
	// [wait] may trail on the same line; take it off first so the bracket
	// scan sees a whole token.
	s, _ = stripBareToken(s, WaitToken)
	s, _ = stripBareToken(s, NoWaitToken)
	_, emoji := stripReactTokens(s)
	return emoji
}

// StripReactTokens removes every [react …] token, own-line or trailing.
func StripReactTokens(s string) string {
	out, _ := stripReactTokens(s)
	return out
}

func stripReactTokens(s string) (string, string) {
	if s == "" || !strings.Contains(strings.ToLower(s), ReactPrefix) {
		return s, ""
	}
	emoji := ""
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if e, ok := wholeReactToken(t); ok {
			if emoji == "" {
				emoji = e
			}
			continue
		}
		if i := strings.LastIndex(strings.ToLower(t), " "+ReactPrefix); i >= 0 {
			if e, ok := wholeReactToken(t[i+1:]); ok {
				if emoji == "" {
					emoji = e
				}
				line = strings.TrimSpace(t[:i])
			}
		}
		keep = append(keep, line)
	}
	return strings.TrimSpace(strings.Join(keep, "\n")), emoji
}

// wholeReactToken reads "[react X]" exactly: prefix, non-empty body, closing
// bracket at the end, nothing after.
func wholeReactToken(t string) (string, bool) {
	lower := strings.ToLower(t)
	if !strings.HasPrefix(lower, ReactPrefix) || !strings.HasSuffix(t, "]") {
		return "", false
	}
	body := strings.TrimSpace(t[len(ReactPrefix) : len(t)-1])
	if body == "" || strings.ContainsAny(body, "[]\n") {
		return "", false
	}
	return body, true
}

// incompleteReactToken is a streaming prefix of a [react …] token: "[re",
// "[react ", "[react 👍" — hidden so the marker never paints on the phone.
func incompleteReactToken(t string) bool {
	lower := strings.ToLower(t)
	if lower == "" || !strings.HasPrefix(lower, "[") {
		return false
	}
	if strings.HasPrefix(ReactPrefix, lower) {
		return true
	}
	return strings.HasPrefix(lower, ReactPrefix) && !strings.Contains(lower, "]")
}
