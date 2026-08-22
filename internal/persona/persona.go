// Package persona loads and concatenates markdown from PERSONA_DIR.
package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// FilePersona is the operator-owned persona file (identity, rules, human, tools).
	FilePersona = "PERSONA.md"
	// FileSelf is the agent-written personality file. Same basename as selfnote.FileName.
	FileSelf = "SELF.md"
)

// PreferredOrder is the concat order for the two persona files.
// Missing files are skipped. Other *.md files are ignored (legacy split is
// deleted by SyncKernel, not loaded).
// PERSONA (who / how / human / tools) → SELF (who it's become; agent-written).
var PreferredOrder = []string{
	FilePersona,
	FileSelf,
}

// LegacyFiles is the pre-collapse split. SyncKernel concatenates these into
// PERSONA.md when that file is missing, then deletes them.
var LegacyFiles = []string{
	"SOUL.md",
	"RULES.md",
	"USER.md",
	"TOOLS.md",
}

// Load reads PERSONA.md then SELF.md and concatenates them.
// A missing directory or empty set of files yields ("", nil) — tolerant by design.
func Load(dir string) (string, error) {
	var parts []string
	for _, name := range PreferredOrder {
		text, err := readOptional(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		if text == "" {
			continue
		}
		text = stampPreferred(name, text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func stampPreferred(name, text string) string {
	switch name {
	case FileSelf:
		return stampSELF(text)
	case FilePersona:
		return stampPersona(text)
	default:
		return text
	}
}

// tzField matches PERSONA.md lines like `- **Timezone:** America/Los_Angeles`
// (colon may sit inside the bold markers).
var tzField = regexp.MustCompile(`(?i)\**timezone\**\s*:\s*\**\s*([A-Za-z0-9_+\-/]+)`)

// Timezone extracts an IANA zone from PERSONA.md-style persona text.
// Empty if the field is missing or not a loadable location.
func Timezone(text string) string {
	m := tzField.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	name := strings.TrimSpace(m[1])
	if name == "" {
		return ""
	}
	if _, err := time.LoadLocation(name); err != nil {
		return ""
	}
	return name
}

// ResolveTimezone prefers PERSONA.md Timezone over fallback (CRON_TZ).
func ResolveTimezone(personaText, fallback string) (name string, loc *time.Location, source string) {
	if tz := Timezone(personaText); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err == nil {
			return tz, loc, FilePersona
		}
	}
	fb := strings.TrimSpace(fallback)
	if fb == "" {
		fb = "America/Los_Angeles"
	}
	loc, err := time.LoadLocation(fb)
	if err != nil {
		loc, err = time.LoadLocation("America/Los_Angeles")
		if err != nil {
			return "UTC", time.UTC, "UTC"
		}
		return "America/Los_Angeles", loc, "America/Los_Angeles"
	}
	return fb, loc, "CRON_TZ"
}

func readOptional(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("persona: read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}
