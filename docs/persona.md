# Persona files

> Pitch: [../readme.md](../readme.md) · Contract: [design.md](design.md) ·
> Grown personality: [troubleshooting.md](troubleshooting.md#selfmd--personality-drift)

Two markdown files in `PERSONA_DIR`. That is the whole standing prompt besides
memory hydration, history, and **this turn’s tool schemas**.

| File | Who writes it | Job |
| --- | --- | --- |
| `PERSONA.md` | You | Who the agent **should** be, who you are, harness-builtin policy |
| `SELF.md` | The agent (`self_note`, distill on `/new`) | Who it **became** with you — voice, jokes, rituals, a few north-star aims |

Boot concatenates them in that order. Extra `*.md` is ignored. Leftover
`SOUL.md` / `RULES.md` / `USER.md` / `TOOLS.md` are merged into `PERSONA.md`
(if it is missing) and then deleted.

The harness overwrites the **Self-notes** and **Location pins** sections in
`PERSONA.md` and the `SELF.md` header. Your bullets stay.

---

## MCP tools are not this file

Loaded MCP servers reach the model from the **live catalog**:

- Tool schemas on the Completer call (`mcp.toml` grant, `mcp_enable` filter)
- `[mcp prefixes]` in the prompt when dynamic tools are on
- `/tools` for you

Do **not** copy `google__…` / `garmin__…` / routing tables into `PERSONA.md`.
That file is in the cached prefix every turn. If Google is not mounted, a
persona that names `google__calendar_list_events` teaches the model to invent
a tool. If Google *is* mounted, the schema already has the name.

`PERSONA.md` may mention harness builtins (`self_note`, `memory_*`, `cron_*`,
`watch_*`, `mcp_enable`) and **your** facts the schema cannot know (timezone,
canonical email, “ask first”). House notes (“bike = bicycling”) belong under
**About you**, not as a fake catalog.

Wiring tools: [mcp.md](mcp.md).

---

## Where the horizon lives

Long-horizon planning is not one file. Mixing a marathon log into `SELF.md`
is the same overspill as putting MCP recipes in `PERSONA.md`.

| Layer | Always in the prompt? | Holds |
| --- | --- | --- |
| `SELF.md` | Yes (cap ~4KB) | Who you became: voice, jokes, rituals, **3–5 north-star sentences** that change how you show up for months. Not mileage, due dates, or this week’s open loops. |
| SQLite memory | Hydrate ≤ ~30 rows (FTS + recency; `insight` sorts last) | The tracker. Months-scale plan: `insight` / subject `aim/<area>`. Progress: `fact`. Recipes: `skill/<area>`. Forget when the aim moves. |
| cron / watch | No — wakes a later turn | The loop. A goal with no wake is a dusty row. |
| Calendar / Tasks (MCP) | No | Dated to-dos with a real due date. |

**Not all SQL.** Hydration is lossy. A six-month aim that was not mentioned
this week can fall out of the 30-row block. A north-star sentence in
`SELF.md` is the always-on reminder that the horizon exists.

**Not all `SELF.md`.** Distill treats notes like personality. Progress needs
forget / FTS / supersede. Jokes competing with project plans is mystery
blending again.

No new `goal` kind. `insight` + `aim/` is the convention. Canonical seed
teaches this: [`examples/persona/PERSONA.example.md`](../examples/persona/PERSONA.example.md).

---

## Writing `PERSONA.md` (keep it tight)

Context is an **attention budget**. Long prompts rot in the middle — the model
follows the first lines and the last, and quietly drops the handbook in
between ([Anthropic, context engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)).
A 16k-character file that restates “don’t bluff” three times is worse than a
3k file with two examples.

**Budget:** aim **2–4k characters** on disk (~500–1k tokens). Kernel stamps add
a bit more at boot. Past that you are competing with history for attention —
and the whole prefix is **re-billed every Completer round**, so a fat persona
makes serial tool loops even more expensive. Personality that stays short
still steers a long chat; a wall of rules becomes background noise around
message 20.

**Altitude:** specific enough to change behavior, not a brittle if-else dump
and not “be helpful.” Testable always/never beats vibes. “2–4 sentences, answer
first” beats “be concise.”

**Examples over edge-case lists.** Two or three canonical shots teach more than
forty rules:

```text
“what’s on today?” → calendar + mail + memory_recall in ONE response
(independent lookups), then two sentences. Never a fake empty calendar.
Never serial calendar-then-mail.
“how’s the long goal going?” → recall `aim/` then live tools. Never invent
progress.
A running joke → quote SELF.md. Don’t paraphrase it.
```

Diverse: one task, one **horizon** (`aim/` recall), one voice. Don’t add an
example that only restates a bullet you already wrote.

**Shape that works:**

1. Identity + voice (who, how it talks)
2. A few examples (include one long-horizon recall)
3. Hard do/don’t (tools-first **and batch independent calls**, identity lock, ask-first)
4. **Memory hygiene** — the three-layer split (north-star / tracker / wake).
   Keep that section; do not grow it into a project plan. Kernel stamps
   Self-notes + Location pins.
5. **About you** (timezone as `- **Timezone:** Area/City`)
6. One load-bearing closer at the **end** (recency): if a tool is in this
   turn’s list, call it; independent lookups all in this response; don’t
   invent live facts

Put the rule that must never slip on the last line. Models weight the last
instruction they saw.

**Cut:** product pitches, duplicate sections, per-server encyclopedias,
anything `/tools` already shows, **progress logs / mileage / open loops**
(those are memory, not persona). If a sentence does not change behavior,
delete it.

Canonical seed: [`examples/persona/PERSONA.example.md`](../examples/persona/PERSONA.example.md)
(`make init`).

---

## `SELF.md`

Agent-written, cap ~4KB, append-only mid-chat, merge on `/new`. You prune it.

Keep **3–5 north-star sentences** (how you show up for months). Move mileage,
dates, and open loops to memory (`aim/<area>`). Audit: [troubleshooting.md](troubleshooting.md#selfmd--personality-drift).
Why not all SQL / not all this file: [Where the horizon lives](#where-the-horizon-lives).
