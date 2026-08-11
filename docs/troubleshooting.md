# Troubleshooting

> Pitch + contract: [../readme.md](../readme.md) · Index: [README.md](README.md)

Operator fixes for common “why is it doing that?” moments. Prefer grepping
logs (`self-notes disabled`, `self distill`, `TOOL_MAX_ITERATIONS`) before
rewriting persona.

## `SELF.md` — personality drift

Gantry’s **self-notes** feature lets the agent grow a personality that
survives `/new`:

- Mid-chat: builtin `self_note` appends a short line to `PERSONA_DIR/SELF.md`.
- On `/new`: a distill pass **rewrites** the whole file from the dying session
  + existing notes (not a blind append).
- Loaded every turn as part of the persona (after `SOUL.md`). Cap ~4KB.

That is the feature — and the footgun. Notes reinforce themselves: a snarky
line becomes “who I am,” then the next distill keeps it. **You own the veto.**

### When to audit

Open `SELF.md` (or wipe it) when:

- The agent feels wrong after a reset — too clingy, too snarky, off-brand jokes
- It keeps referencing a game / nickname / bit you are done with
- A long tool-heavy or argumentative session just ended (distill may have
  locked in a bad mood)
- You shared the allowlist with someone else and want a clean slate for them

### How to fix

```bash
# Host path (compose consumer / fleet)
$EDITOR ./persona/SELF.md

# Inside the container
docker compose exec gantry cat /persona/SELF.md
```

| Action | Effect |
| --- | --- |
| Delete specific `-` lines | Surgical prune; next turn loads the shorter file |
| Truncate to header only / delete file | Fresh personality; next `/new` can grow it again |
| `SELF_NOTES_ENABLED=false` | Disables `self_note` + distill; persona can stay `:ro` |
| Mount `./persona:/persona:ro` | Same as disable — boot logs `self-notes disabled` |

After editing on the host, the running process still has the old persona in
memory until:

- The next `self_note` / distill (reloads persona after write), or
- `SIGHUP` (native), or
- Container recreate / process restart

### Sell vs safety (read both)

**Why keep it on:** without `SELF.md`, the “funny agent” lives only in
Telegram history. `/new` or a context blow-up lobotomizes them. Self-notes
are how ownership means *continuity*, not just *control*.

**Why audit:** the agent cannot violate `RULES.md` by design of the prompt,
but tone and rituals are soft. Undesirable personality is an ops problem —
treat `SELF.md` like a log you review, not a sacred file you never open.

Related: [../local-agent/docs/persona.md](../local-agent/docs/persona.md) ·
[security.md](security.md).

## Self-notes silently off

**Symptom:** `/tools` has no `self_note`; `/new` always says plain
`session reset` (never “personality distilled…”); boot log has
`self-notes disabled (persona dir not writable)`.

**Cause:** `PERSONA_DIR` is not writable — almost always Docker
`./persona:/persona:ro` or a read-only host directory.

**Fix:**

1. Mount persona writable: `./persona:/persona` (no `:ro`).
2. Ensure the container user can write the host dir (UID/GID, e.g. fleet
   `GANTRY_UID` / `GANTRY_GID`).
3. Or set `SELF_NOTES_ENABLED=false` if you want read-only persona on purpose.

Compose templates in this repo mount persona writable by default for this
reason.

## Agent “forgot” who it was after `/new`

**Expected without self-notes:** session history + rolling summary are wiped;
only `SOUL` / `RULES` / `USER` / `TOOLS` remain → bland reboot.

**With self-notes enabled + writable persona:** `/new` should reply
`session reset — personality distilled into SELF.md` after a session of at
least a few turns. Check:

1. Boot log: self-notes ready vs disabled.
2. `SELF.md` on disk after `/new` — did it grow?
3. Distill is best-effort: if the LLM call fails, reset still happens and the
   log shows `self distill: … failed`. Personality from that session is lost;
   prior `SELF.md` content is kept.

## Agent keeps calling tools forever / burns tokens

Default tool budget is `TOOL_MAX_ITERATIONS=10` tool rounds, then a **landing
call with tools withheld** so the turn ends in a real reply (and history is
saved) instead of an error that drops the work. A soft warning fires around
70% of the budget.

If you still see anonymous `✓ ✓ ✓ …` with no narration, check `TOOL_TRACE`
and whether the model is emitting a one-line reason before each call (persona
nudges this). Raise or lower the budget via env; see [design.md](design.md).

## More

| Topic | Doc |
| --- | --- |
| Memory rows wrong | [memory.md](memory.md) |
| MCP name / auth failures | [mcp.md](mcp.md) · [auth.md](auth.md) |
| Slow local turns | [deploy-native.md](deploy-native.md) · [observability.md](observability.md) |
| Threat model | [security.md](security.md) |
