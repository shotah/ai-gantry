package mcp

import (
	"fmt"
	"strings"
	"unicode"
)

const maxToolNameLen = 64

// PrefixedName builds the OpenAI-safe tool name: "{server}__{tool}".
func PrefixedName(server, tool string) (string, error) {
	s := sanitizeName(server)
	t := sanitizeName(tool)
	if s == "" || t == "" {
		return "", fmt.Errorf("mcp: empty tool name after sanitize (server=%q tool=%q)", server, tool)
	}
	name := s + "__" + t
	if len(name) > maxToolNameLen {
		return "", fmt.Errorf("mcp: tool name %q exceeds %d chars (OpenAI limit)", name, maxToolNameLen)
	}
	return name, nil
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
