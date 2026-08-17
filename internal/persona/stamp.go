package persona

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/shotah/ai-gantry/internal/selfnote"
)

func stampSELF(raw string) string {
	return selfnote.Stamp(raw)
}

func stampRules(raw string) string {
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

// SyncKernel writes kernel RULES sections (Self-notes, Location pins) to disk
// when the file is writable. Best-effort: a read-only mount leaves the prompt
// stamp (Load) in place and returns the write error.
func SyncKernel(dir string) error {
	path := filepath.Join(dir, "RULES.md")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	next := stampRules(string(b))
	if strings.TrimSpace(string(b)) == next {
		return nil
	}
	return os.WriteFile(path, []byte(next+"\n"), 0o644)
}
