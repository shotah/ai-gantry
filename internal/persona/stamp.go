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
	if raw == "" {
		return selfnote.RulesSection
	}
	if start, end, ok := selfNotesSpan(raw); ok {
		return strings.TrimSpace(raw[:start] + selfnote.RulesSection + "\n\n" + raw[end:])
	}
	const mem = "## Memory hygiene"
	if i := strings.Index(raw, mem); i >= 0 {
		return strings.TrimSpace(raw[:i] + selfnote.RulesSection + "\n\n" + raw[i:])
	}
	return raw + "\n\n" + selfnote.RulesSection
}

// selfNotesSpan is the ## Self-notes section through the next ## heading or EOF.
func selfNotesSpan(raw string) (start, end int, ok bool) {
	const head = "## Self-notes"
	start = indexHeading(raw, head)
	if start < 0 {
		return 0, 0, false
	}
	rest := raw[start+len(head):]
	if rel := nextHeading(rest); rel >= 0 {
		return start, start + len(head) + rel, true
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

// SyncKernel writes the current RULES.md Self-notes section to disk when the
// file is writable. Best-effort: a read-only mount leaves the prompt stamp
// (Load) in place and returns the write error.
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
