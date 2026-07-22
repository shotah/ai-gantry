// Package persona loads and concatenates markdown from PERSONA_DIR.
package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// preferredOrder is the fixed concat order for well-known persona files.
// Missing files are skipped. Any other *.md files follow alphabetically.
var preferredOrder = []string{
	"SOUL.md",
	"IDENTITY.md",
	"USER.md",
	"AGENTS.md",
	"TOOLS.md",
	"HEARTBEAT.md",
	"BOOTSTRAP.md",
	"MEMORY.md",
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

	preferred := make(map[string]struct{}, len(preferredOrder))
	var parts []string
	for _, name := range preferredOrder {
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

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("persona: read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}
