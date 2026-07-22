package mcp

import "unicode/utf8"

// Truncate limits s to maxChars runes, appending a marker when cut.
// maxChars <= 0 means no truncation.
func Truncate(s string, maxChars int) string {
	if maxChars <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	const marker = "\n…[truncated]"
	keep := maxChars - utf8.RuneCountInString(marker)
	if keep < 1 {
		keep = 1
	}
	runes := []rune(s)
	if keep > len(runes) {
		keep = len(runes)
	}
	return string(runes[:keep]) + marker
}
