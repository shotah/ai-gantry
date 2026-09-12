package agent

import "strings"

// harnessClockNote labels the per-turn clock so Completer RoleUser stays
// speech. Trailing unlabeled RoleSystem was easy to skip; leading with
// [current time] primed calendar/tool fixation. After their words, tagged.
const harnessClockNote = "[harness] Not user text — location, clock, and hours for this turn."

func formatHarnessClock(clock string) string {
	clock = strings.TrimSpace(clock)
	if clock == "" {
		return ""
	}
	return harnessClockNote + "\n" + clock
}

// stripHarnessContext drops pasted / old-client clock and hydration blocks
// from inbound text so they are not stored or used as the hydrate query.
func stripHarnessContext(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "\n\n")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if harnessBlock(p) {
			continue
		}
		trimmed := strings.TrimRight(stripTrailingHarnessLines(p), "\n")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.TrimSpace(strings.Join(kept, "\n\n"))
}

func harnessBlock(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return harnessTagLine(line)
	}
	return false
}

func stripTrailingHarnessLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if !harnessTagLine(strings.TrimSpace(line)) {
			continue
		}
		if i == 0 {
			return s
		}
		return strings.Join(lines[:i], "\n")
	}
	return s
}

func harnessTagLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "[harness]"),
		strings.HasPrefix(line, "[location"),
		strings.HasPrefix(line, "[current time]"),
		strings.HasPrefix(line, "[hours]"),
		strings.HasPrefix(line, "[memory]"):
		return true
	default:
		return false
	}
}
