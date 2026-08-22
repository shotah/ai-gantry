package persona

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shotah/ai-gantry/internal/selfnote"
)

func stampSELF(raw string) string {
	return selfnote.Stamp(raw)
}

func stampPersona(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = upsertSection(raw, "## Self-notes", selfnote.RulesSection)
	raw = upsertSection(raw, "## Location pins", selfnote.LocationSection)
	return strings.TrimSpace(raw)
}

func upsertSection(raw, heading, body string) string {
	if raw == "" {
		return body
	}
	if start, end, ok := sectionSpan(raw, heading); ok {
		return strings.TrimSpace(raw[:start] + body + "\n\n" + raw[end:])
	}
	const mem = "## Memory hygiene"
	if i := strings.Index(raw, mem); i >= 0 {
		return strings.TrimSpace(raw[:i] + body + "\n\n" + raw[i:])
	}
	return raw + "\n\n" + body
}

// sectionSpan is heading through the next ## heading or EOF.
func sectionSpan(raw, heading string) (start, end int, ok bool) {
	start = indexHeading(raw, heading)
	if start < 0 {
		return 0, 0, false
	}
	rest := raw[start+len(heading):]
	if rel := nextHeading(rest); rel >= 0 {
		return start, start + len(heading) + rel, true
	}
	return start, len(raw), true
}

func indexHeading(raw, heading string) int {
	if strings.HasPrefix(raw, heading) {
		return 0
	}
	i := strings.Index(raw, "\n"+heading)
	if i < 0 {
		return -1
	}
	return i + 1
}

func nextHeading(raw string) int {
	if i := strings.Index(raw, "\n## "); i >= 0 {
		return i + 1
	}
	return -1
}

// SyncKernel migrates leftover SOUL/RULES/USER/TOOLS into PERSONA.md when that
// file is missing, deletes those legacy files, then writes kernel sections
// (Self-notes, Location pins) into PERSONA.md. Best-effort: a read-only mount
// leaves the prompt stamp (Load) in place and returns the write/remove error.
func SyncKernel(dir string) (removed []string, err error) {
	removed, err = reconcileLegacy(dir)
	if err != nil {
		return removed, err
	}
	if err := stampPersonaFile(dir); err != nil {
		return removed, err
	}
	return removed, nil
}

func stampPersonaFile(dir string) error {
	path := filepath.Join(dir, FilePersona)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	next := stampPersona(string(b))
	if strings.TrimSpace(string(b)) == next {
		return nil
	}
	return os.WriteFile(path, []byte(next+"\n"), 0o644)
}

func reconcileLegacy(dir string) ([]string, error) {
	if err := migrateLegacy(dir); err != nil {
		return nil, err
	}
	return removeLegacy(dir)
}

func migrateLegacy(dir string) error {
	personaPath := filepath.Join(dir, FilePersona)
	_, err := os.Stat(personaPath)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("persona: stat %s: %w", personaPath, err)
	}

	var parts []string
	for _, name := range LegacyFiles {
		text, err := readOptional(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("persona: mkdir %s: %w", dir, err)
	}
	body := strings.Join(parts, "\n\n") + "\n"
	if err := os.WriteFile(personaPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("persona: migrate %s: %w", personaPath, err)
	}
	return nil
}

func removeLegacy(dir string) ([]string, error) {
	var (
		removed []string
		errs    []error
	)
	for _, name := range LegacyFiles {
		path := filepath.Join(dir, name)
		err := os.Remove(path)
		if err == nil {
			removed = append(removed, name)
			continue
		}
		if os.IsNotExist(err) {
			continue
		}
		errs = append(errs, fmt.Errorf("persona: remove %s: %w", path, err))
	}
	return removed, errors.Join(errs...)
}
