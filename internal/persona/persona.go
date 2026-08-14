// Package persona loads and concatenates markdown from PERSONA_DIR.
package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// PreferredOrder is the fixed concat order for well-known persona files.
// Missing files are skipped. Any other *.md files follow alphabetically.
// Keep this lean: SOUL (who) → SELF (who it's become; agent-written) →
// RULES (how) → USER (human) → TOOLS (MCP recipes).
var PreferredOrder = []string{
	"SOUL.md",
	"SELF.md",
	"RULES.md",
	"USER.md",
	"TOOLS.md",
}

// Load reads markdown files from dir and concatenates them in fixed order.
// A missing directory or empty set of files yields ("", nil) — tolerant by design.
func Load(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("persona: read dir %s: %w", dir, err)
	}

	available := make(map[string]struct{}, len(entries))
	var extras []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		available[name] = struct{}{}
		extras = append(extras, name)
	}
	sort.Strings(extras)

	preferred := make(map[string]struct{}, len(PreferredOrder))
	var parts []string
	for _, name := range PreferredOrder {
		preferred[name] = struct{}{}
		if _, ok := available[name]; !ok {
			continue
		}
		text, err := readFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	for _, name := range extras {
		if _, ok := preferred[name]; ok {
			continue
		}
		text, err := readFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, "\n\n"), nil
}

// tzField matches USER.md lines like `- **Timezone:** America/Los_Angeles`
// (colon may sit inside the bold markers).
var tzField = regexp.MustCompile(`(?i)\**timezone\**\s*:\s*\**\s*([A-Za-z0-9_+\-/]+)`)

// Timezone extracts an IANA zone from USER.md-style persona text.
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

// ResolveTimezone prefers USER.md Timezone over fallback (CRON_TZ).
func ResolveTimezone(personaText, fallback string) (name string, loc *time.Location, source string) {
	if tz := Timezone(personaText); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err == nil {
			return tz, loc, "USER.md"
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

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("persona: read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}
