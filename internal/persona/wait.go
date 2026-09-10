package persona

// WaitSection is the kernel-owned follow-up block injected into PERSONA.md.
// Operators do not edit this; gantry overwrites the section on load/sync.
const WaitSection = "## Follow-up (`[wait]`)\n\n" +
	"- **Not a tool.** If you asked a question they should answer, put `[wait]` on its own line (stripped before they see it). Two follow-up pokes if they ghost, then stop.\n" +
	"- `[nowait]` on its own line drops the wait. `[conversation] waiting_for_reply=true` → do not ask a new different question.\n" +
	"- Jokes, statements, and done-for-now replies: no `[wait]`."
