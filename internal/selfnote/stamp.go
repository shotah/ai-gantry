package selfnote

import "strings"

// RulesSection is the kernel-owned Self-notes block injected into PERSONA.md.
// Operators do not edit this; gantry overwrites the section on load/sync.
const RulesSection = "## Self-notes (`self_note` → SELF.md)\n\n" +
	"- **Append-only.** One short `-` line. Does **not** rewrite `SELF.md`.\n" +
	"- Skip if the vibe or north-star is already in the `SELF.md` bullets in this prompt.\n" +
	"- Personality / jokes / rituals / a few north-star aims only — not facts, progress logs, or tool recipes.\n" +
	"- A north-star is one sentence that changes how you show up for months. Progress, dates, and open loops are memory_store. A one-off to-do is memory or cron.\n" +
	"- Empty board: no north-star in SELF.md and no aim/ in memory → ask ONE months-scale question. Do not invent. After they answer: self_note + memory_store insight subject aim/<area>. Ask at most once per day (memory subject aim/bootstrap).\n" +
	"- Prefer exact joke wording and nicknames in SELF.md. A vibe word is not a joke.\n" +
	"- Do this **unprompted** when a vibe or north-star lands. `/new` distill merges (does not flatten jokes)."

// LocationSection is the kernel-owned GPS block injected into PERSONA.md.
const LocationSection = "## Location pins\n\n" +
	"- Pendant GPS and Telegram location/venue update this chat's last known `[location]` (in-memory). A GPS-only frame with no chat text updates that cache and does not start a turn.\n" +
	"- `[location]` is on this turn's user-line time footer (after their words; they did not type it), with when it was from. Use those `lat,lng` for maps `near` or route origin. Do not invent a city.\n" +
	"- just now / minutes: you have a current fix. Hours old: still their last known place — say so if a live pin matters, otherwise use it. Missing: no GPS this process; ask them to send with GPS on (pendant) or a Telegram location. Do not guess."

// Body returns SELF.md without the kernel header (title + leading blockquotes).
// Operator/agent bullets are kept. An old header is dropped so Stamp can
// replace it.
func Body(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	i := 0
	if looksLikeSelfTitle(lines[0]) {
		i++
	}
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, ">") {
			i++
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines[i:], "\n"))
}

func looksLikeSelfTitle(line string) bool {
	t := strings.TrimSpace(strings.TrimLeft(line, "#"))
	t = strings.TrimSpace(t)
	return strings.Contains(strings.ToLower(t), "self.md")
}

// Stamp returns Header plus the file body. Empty body → Header only.
func Stamp(raw string) string {
	body := Body(raw)
	if body == "" {
		return Header
	}
	return Header + "\n\n" + body
}
