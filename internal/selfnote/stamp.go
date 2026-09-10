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

// LocationSection is the kernel-owned last-pin block injected into PERSONA.md.
const LocationSection = "## Location pins\n\n" +
	"- Pendant GPS (every send, or a silent drop-pin) and Telegram location/venue update this chat's **last pin**.\n" +
	"- Coords lead this turn's time footer as `[last pin]` (after their words; they did not type them). Use that for maps, places, directions, \"near me.\" Do not invent a city.\n" +
	"- If `[last pin]` is present and recent (just now / minutes / ±accuracy), you have their location. Pass `lat,lng` to maps `near` or route origin. Do not ask for a Telegram pin.\n" +
	"- If it is missing or hours old, ask them to drop a pin (pendant: GPS on; or a Telegram location). Do not guess.\n" +
	"- A bare pin (no chat text) only updates the cursor — wait for their ask."

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
