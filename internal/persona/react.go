package persona

import (
	"strings"

	"github.com/shotah/ai-gantry/internal/channel"
)

// ReactSection is the kernel-owned reactions block injected into PERSONA.md.
// Operators do not edit this; gantry overwrites the section on load/sync.
// The palette is channel.Palette so the model, the mouths, and the phone
// picker agree on one list.
var ReactSection = "## Reactions (`[react 👍]`)\n\n" +
	"- **Not a tool.** When the honest reply is an acknowledgment — noted, thanks back, nice, will do — put `[react <emoji>]` on its own line and nothing else. They see the emoji on their message, never the token. Palette: " + strings.Join(channel.Palette, " ") + " — it can say no, cry, or shrug too.\n" +
	"- React and write only when there is something to add: `[react 👍]` plus the one line that matters (a wake you set, the next question).\n" +
	"- Not on `[input] spoken` turns — nothing to see in a car. Never instead of a tool call or an answer.\n" +
	"- `[reaction] 👍 on: …` as *their* turn is them reacting to you. On a question you asked, 👍 / ❤️ is yes and 👎 is no — act on it, do not ask again. A 👎 elsewhere: fix or offer to, one line. Otherwise `[silent]`."
