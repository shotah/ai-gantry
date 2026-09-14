package memory

import "strings"

// Subject prefixes stamped onto [harness] every turn (hydration is lossy).
const (
	SubjectAimPrefix     = "aim/"
	SubjectWaitingPrefix = "waiting/"
	SubjectFollowPrefix  = "follow/"
	harnessHorizonMax    = 5
	harnessHorizonClip   = 72
)

// FormatAims is the per-turn [aims] line. Empty if there are no live aim/ rows.
// North-star sentences stay in SELF.md; this is the tracker (insight + aim/<area>).
func FormatAims(entries []Entry) string {
	parts := horizonParts(entries, SubjectAimPrefix, true)
	if len(parts) == 0 {
		return ""
	}
	return "[aims] " + strings.Join(parts, " · ")
}

// FormatLoops is the per-turn [loops] line for this-week open loops
// (waiting/ and follow/ facts). Empty if none.
func FormatLoops(entries []Entry) string {
	parts := horizonParts(entries, "", false)
	if len(parts) == 0 {
		return ""
	}
	return "[loops] " + strings.Join(parts, " · ")
}

func horizonParts(entries []Entry, stripPrefix string, strip bool) []string {
	if len(entries) == 0 {
		return nil
	}
	if len(entries) > harnessHorizonMax {
		entries = entries[:harnessHorizonMax]
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		label := strings.TrimSpace(e.Subject)
		if strip && stripPrefix != "" {
			label = strings.TrimPrefix(label, stripPrefix)
		}
		if label == "" {
			continue
		}
		body := clipHorizon(e.Content)
		if body == "" {
			parts = append(parts, label)
			continue
		}
		parts = append(parts, label+": "+body)
	}
	return parts
}

func clipHorizon(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= harnessHorizonClip {
		return s
	}
	return string(r[:harnessHorizonClip-1]) + "…"
}
