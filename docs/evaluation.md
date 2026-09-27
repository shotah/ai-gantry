# Harness evaluation

*Authored by Claude Fable 5.1 (Anthropic), working in Cursor, September
2026; revised 2026-09-26 after the aims ledger
([aims-progress.md](aims-progress.md)) and the tasks list
([tasks.md](tasks.md)) shipped. The opinions below — including the
verdict — are the model's, not the maintainer's. Where the maintainer
corrected a misread design choice, the correction is folded in, not
footnoted.*

An honest read of `gantry` as an AI harness — what it is good at, where it
is thin, and how it sits next to the other self-hosted personal-agent
harnesses people actually run in 2026. Written from the tree, the test
suite, the Completer goldens, and the live behavioral eval, not from the
pitch. Inventory is [features.md](features.md); the loop is
[architecture.md](architecture.md); open work is [todo.md](todo.md).

Comparators were checked against their own docs on 2026-09-14:
[OpenClaw](https://docs.openclaw.ai/), [Letta Code](https://github.com/letta-ai/letta-code)
(and the [March 2026 direction post](https://www.letta.com/blog/our-next-phase/)),
[Hermes Agent](https://hermes-agent.nousresearch.com/docs/). This is a
design comparison, not a benchmark. Nobody here ran the four side by side
on the same tasks.

---

## Overall impression

Gantry is a **disciplined, single-purpose harness**: one static Go binary,
one persona, one OpenAI-compat socket, optional MCP children, SQLite. It
refuses most of what the category is currently adding (UIs, routers,
subagents, skills marketplaces, inbound ports) and spends that budget on
six things the others mostly leave to the model — or do not do at all:

1. **What the model actually sees.** The prompt is a pinned contract. PWA
   inbound JSON → `Handle` → Completer request → HTTP body are all
   goldens, and the wire golden is the *same file* the agent-side test
   pins. When the harness stamps time, GPS, hours, aims, loops, wakes,
   surface, and last contact, the test suite proves the bytes land — on
   OpenAI-layout *and* on Gemini's one-system-instruction layout.
2. **Small-model tool hygiene.** Prefix alias, closest-name candidates,
   grammar-constrained retry, printed-call salvage, CoT promotion, landing
   call, thought-signature plumbing, result collapse by tool round. This
   is the unglamorous layer that makes a 4–30B local model usable as an
   agent instead of a demo — and `/toolstats` counts every repair.
3. **The tool catalog as the procedure.** Intent-first tool descriptions
   are the recipe; `[mcp prefixes]` publishes the catalog by name and
   ships schemas only for prefixes the chat enabled; fiddly exceptions
   live as `skill/<area>` memory rows recalled on demand. No skills
   directory, because it would duplicate the manifest.
4. **Personality that survives reset, with an operator veto.** `SELF.md`,
   Voice ledger on fold, distill on `/new`, and a plain text file you can
   prune. Facts go to SQLite, not into the persona.
5. **A mouth the model can dress.** On the pendant and cab the agent
   owns its own face, the chat wallpaper, and the room's color mood —
   `pendant__avatar_update` / `backdrop_update` / `theme_update` from a
   picture it generated and a closed theme catalog it listed. The
   household owns the whole path (crane → Worker → every socket). No
   comparator gives the agent control of the client it is talked to in.
6. **Progress the model does not have to remember.** Months-scale aims
   are one sentence each in memory; what happened against them is a
   ledger the agent writes (`aim_log`: one event, the agent's `-3…+3`
   opinion toward every aim it touches, in the human's words) and the
   kernel does the arithmetic on — 30-day and 7-day ratings on `[aims]`
   every turn, a five-day grid by date on `[progress]` for the planner,
   week buckets and a slope on Sunday, and a ladder (`praised`,
   `nudged`, `asked`, `offered`, `quiet`) that stops the daily planner
   from saying the same line twice. Beside it, the pocket list:
   `todo/<slug>` rows the agent keeps, stamped `[todo]` with ids, that
   never age out and that only the human closes. Both reach the phone
   as mailbox frames (`aims`, `todo`) and as one slash each (`/aims`,
   `/todo`) whose only kernel writes are `block` and `done`. No new
   table for tasks, no target curve for aims, no second model call for
   either.

Behind those six sits a **paid behavior contract** next to the free byte
contract: scenario fixtures replayed against the shipped persona on the
live model, with real MCP catalogs where the fixture names a server. The
goldens say what the model was shown; the eval says what it did with it.
Details in [The behavior contract](#the-behavior-contract).

The cost of that discipline: no authorization layer beyond the allowlist
and the manifest, a harness stamp of twelve possible tags that needs
watching, scores that are an opinion and not a meter, and a behavior gate
that costs money to run — so it gates releases, not pull requests. Channel breadth, skills files, a synthesized user model, memory
auto-save, in-process model routing, and voice are *not* gaps; they are
declined, and the reasons are in [Declined on purpose](#declined-on-purpose).

**Verdict (the model's, per the byline):** best-in-class at *harness-side
context*, at running well on weak models, at keeping the tool catalog
cheap, and — since the ledger — at *following through on a goal over
months without re-asking* among the comparators read above; deliberately without breadth
inside the unit (channels, UI, routers, subagents). Right choice for one
person, one mouth, one brain on a hardened small box, with a phone and a
car screen the agent can make its own — and, as a fleet of those units
under Gantree, for many people, each with their own. Wrong choice for a
shared gateway, for anyone who wants one process serving a crowd, or for
anyone who wants the agent to author its own tooling; the coding-agent
shape is where the rest of the field is pointed, and this steers clear
of it on purpose.

---

## Scorecard

| Area | Verdict | Why |
| --- | --- | --- |
| Prompt assembly & visibility | **Strong** | Byte-pinned goldens from mouth JSON to HTTP body. `[harness]` stamp names only present tags. Gemini fold order tested. |
| Time / place grounding | **Strong** | NOW, day-part, week grid with ISO dates, GPS with fix age, hours, next wakes — no tool call needed. |
| Small-model tool loop | **Strong** | Repairs instead of erroring; ≤5 closest-name candidates then a constrained retry; parallel batch; `/perf` shows trajectory shape; `/toolstats` shows repair counts. |
| Tool catalog cost | **Strong** | `[mcp prefixes]` is byte-stable (cacheable); `mcp_enable` TTL holds (27h / 6h); `tools` / `exclude` / `tools_prefix` filters; payloads from rounds older than the last 2 tool rounds collapse, so a parallel batch is always read back whole. |
| Procedural memory | **Good, different shape** | Manifest + descriptions are the recipe; `skill/<area>` rows for exceptions; examples seed teaches the pattern. Depends on the model choosing to store — consistent with auto-save off. |
| Personality persistence | **Strong** | `SELF.md` + Voice fold + distill; operator prune; `:ro` kill switch. |
| Proactivity | **Strong** | Daily planner (one clock time, `[silent]` on a day off), examples, cron, quiet watches. The planner reads `[aims]` / `[todo]` / `[progress]` / `[wakes]` instead of re-fetching, logs yesterday from the tools it already called, schedules today's cues pinned to the aim or task they serve, and climbs a ladder keyed on the ledger so the nudge is never yesterday's sentence. |
| Goal tracking | **Strong, opinionated** | Event ledger (`aim_event` / `aim_score` / `aim_block`), agent-scored `-3…+3` against a fixed rubric and per-domain anchors, kernel-computed rating, grid, weeks, slope, Pearson on measurement vs effort. Evidence gate in the eval: every logged `what` must trace to a tool result or the human's words. The score is the agent's opinion by design; the human argues in chat and re-scores by id. |
| Tasks | **Good, small on purpose** | `todo/<slug>` memory rows, no table. `[todo]` with ids every turn; the planner parses the words against the week grid and cron-schedules the cue on the task's own day. Never ages out, never offered for dropping; only the human closes one. Whether the nag actually varies day to day is asserted by one planner fixture, not measured over a week. |
| Long-term memory (facts) | **Good** | Typed SQLite + FTS5, inspectable with `sqlite3`, persona precedence, consolidator. No embeddings by choice. |
| User model | **Deliberate** | Captured at fold (`Facts:`), not re-derived per turn. Per-turn cost is ≤30 FTS rows keyed on the user's words, no model call. |
| Memory capture | **Deliberate** | Explicit `memory_store` only; the fold is the one compaction call. No flush turn, no auto-save. See [How memory gets written](#how-memory-gets-written). |
| Channels | **Different model** | One hardened host per mouth, not one gateway holding every messenger credential. Telegram (production), Discord, Slack, pendant, stdio. |
| Model-controlled client | **Unique** | Face, wallpaper, and theme on the pendant / cab are the agent's to set (`pendant-mcp`, 7 tools). Picture handoff by `source_path`; theme from a closed catalog with mood lines; humans can unfollow. `[room]` on `[harness]` shows the current look with ages so the model — and the daily planner — redress it without being asked. |
| Security / authorization | **Thin** | Allowlist + manifest-is-grant. No per-tool approval; ask-first is prompt text. |
| Ops surface | **Strong for one box** | No inbound port, Distroless, `gantry status` heartbeat, chat is the console, `/auth` headless OAuth. Fleet ops is gantree, not here. |
| Multi-model / multi-agent | **Absent by design** | No router, no fallback, no subagents inside the unit. One process = one brain; the fleet is Gantree's. |
| Behavioral regression | **Strong, paid** | Twenty-seven scenario fixtures replayed against the shipped seed on the live model, two on real MCP catalogs pulled at run time; shape asserted (tools, args, call counts, `[wait]`, `[silent]`, rows, jobs, prices from tools, ledger evidence, cron `when` against a frozen clock), every run must pass. Fixtures can seed a ledger and aged memory rows and refer to their ids. Release gate and on demand, not per push. One model per reading. |
| Token accounting | **Okay** | chars/4 estimates plus native `usage` when sent. `/tokens` catches fat schemas, not billing. |

---

## Where gantry is ahead

| Dimension | Gantry | OpenClaw | Letta Code | Hermes Agent |
| --- | --- | --- | --- | --- |
| Runtime | One static Go binary, `CGO_ENABLED=0`, Distroless, no inbound port | Node 22+ Gateway with Control UI, binds a port | Harness + agents stored in Letta Cloud for multi-computer; local server option | Python agent + gateway process, seven terminal backends |
| Prompt contract | Goldens from mouth JSON to HTTP body; wire == agent layout by test | Not published as a pinned artifact | Not published as a pinned artifact | Not published as a pinned artifact |
| Behavior contract | Fixtures on the live model against the shipped seed; real MCP catalogs from latest releases; shape and spend asserted | Not published | Trajectory export | Trajectory export, RL tooling |
| Harness-side context (no tool call) | `[current time]` week grid, `[location]` with fix age, `[hours]`, `[aims]` with ratings, `[todo]` with ids, `[loops]`, `[progress]` on the planner, `[wakes]`, `[surface]`, `[room]`, `[last contact]` | Model reads workspace files; cron in gateway | Memory blocks in context (persona / human / custom) | `MEMORY.md` / `USER.md` loaded at session start; skills by name |
| Goals and tasks | Agent-scored event ledger, kernel math (rating, grid, weeks, slope, correlation), ladder that never repeats a note; tasks as memory rows with the planner as due-date parser; both on the phone as frames | Not a harness feature in the docs read; the model keeps notes in workspace files | Not a harness feature in the docs read; memory blocks the agent rewrites | Not a harness feature in the docs read; `MEMORY.md` the model edits |
| Catalog disclosure | `[mcp prefixes]` on/off by server prefix, byte-stable; `mcp_enable` ships schemas next call with a TTL; force list for always-on | Full toolset per agent; skills by name | Tools + skills; skill text loaded on demand | Skill *name* in prompt, `skill_view` loads the file; 60+ builtin tools always on |
| Procedure lives in | Tool descriptions (manifest is the grant and the recipe) + `skill/<area>` memory rows | Workspace Markdown, skills | Skills + memory blocks, MemFS | `SKILL.md` files written by a background review agent, curated by a Curator |
| Weak-model tool repair | Alias, ≤5 closest names, grammar-constrained retry, salvage, CoT promote, landing call; counted in `/toolstats` | Assumes capable model ("use the strongest latest-generation model") | Model-agnostic, frontier-oriented | Model-agnostic; RL / trajectory tooling for training tool-callers |
| In-turn context bounding | Last 2 tool rounds in full, whatever a round asked for; older rounds → one-line marker, args stubbed, signatures kept | Compaction at threshold | Compaction / MemFS | Compaction |
| Personality across reset | `SELF.md` Voice ledger, distill, operator prune | `IDENTITY.md` static; memory flush before compaction | Persona memory block, agent rewrites it | `SOUL.md` static voice; skills carry procedure |
| Agent controls its client | Pendant + cab: own face (`avatar_update`), chat wallpaper (`backdrop_update`), room mood from a closed theme catalog (`theme_list` → `theme_update`); Durable Object broadcasts to every socket | Canvas on companion apps — the agent renders *content* on a surface; client identity and theme are the user's | Chat at chat.letta.com / desktop app; no agent control of the client | TUI + messaging; no agent control of the client |
| Event watches | Cursor + poll on an MCP fetch tool; Completer only on new ids; per-server call budgets floor the interval | Cron / webhooks | — | Cron |
| Metered-API discipline | `budget = "N/day"` per server enforced in the host on every path; refusals name the reset; eval gates one search per ask | — | — | — |
| Memory inspectability | `sqlite3 gantry.db`, typed rows, no vector SaaS | Markdown files + SQLite index | Memory blocks; MemFS git-backed context repo | Markdown + FTS5 session DB + external Honcho |

Five of these matter most. **The prompt contract** — nobody else ships a
test that diffs the socket bytes against the agent layout; it is what
turns a stamp change into a diff instead of a feeling. **The harness
stamp** — time, place, hours, horizon delivered without a tool round.
**Catalog disclosure by prefix** — Hermes discloses skill names and loads
text on demand; gantry does the same thing one level down, at the tool
schema, which is where the tokens actually are, and the on/off line is
stable enough for prefix caching. **The agent's own client** — the others
meet the user in someone else's app (Telegram, WhatsApp, a TUI) or a
vendor web chat, so the agent has a name and a text bubble. On the
pendant and cab the agent has a face it drew, a wallpaper it picked for
the hour, and a color mood it chose to match how the day is going — and
when it changes one, every open phone and car screen repaints. The
plumbing is what makes it hold up: the picture never crosses the model as
bytes (`source_path` from `image__photo_generate`, encoded to budget in
the MCP), the theme is an id from a catalog with a mood line (no invented
hex), the wallpaper has a `delete`, and a human can unfollow and keep
their own theme. It is the connection feature; the `[room]` stamp is what
makes it a habit rather than a party trick. **The ledger** — the
comparators hold a goal as text the model rereads, which means the model
also has to remember how it has been going and what it said about it
last time. Here the agent writes one scored line per event and the
kernel carries everything derived: the 30-day mean on the stamp, the
five-day grid, the week means, the slope, whether a `praised` note went
out this week. The split is the same one memory made — judgment stays
with the model, arithmetic and recall go to SQLite — and it is what lets
a small model run a months-long plan without a second "coach" call. The
eval's `evidence_from_turn` gate is the piece that makes it trustworthy:
a ledger the model could fill from imagination would be worse than none,
so every `what` has to share words with a tool result or the human.
Tasks reuse the pattern at a smaller size — the row is the words, the
planner is the parser, the cue is a cron pinned to the row — and the
decision that nothing on the list expires is the one that keeps it a
to-do list rather than a decay function.

Why the skills comparison flips: Hermes and Letta need recipe files
because their tool surface is generic (bash, browser, a fixed builtin
set) — the procedure has nowhere else to live. Gantry's tool surface is a
typed MCP catalog whose descriptions lead with intent and document hot
paths ([mcp-naming.md](mcp-naming.md)). A skill that says "call
`google__calendar_list_events` with `time_min`" *is* the schema, so a
skills directory would be a second copy of the manifest that could drift.
What remains is the exception path: `skill/<area>` rows with exact names
and one pitfall, recalled before guessing — the examples seed teaches it,
and `mcp-naming.md` lists a skill that cites a non-host tool name as an
anti-pattern.

---

## The behavior contract

The goldens are the best prompt-bytes contract in the category, and they
say nothing about what the model *does*. The eval is the second, paid
contract beside the free one. Each row of the scenario table in
[persona_doc_goals.md](persona_doc_goals.md#scenario-checks) is a fixture:
one turn — a human message, a daily planner wake, or a scheduled-job wake —
against the **shipped seed**, with the production store stack in a temp
dir, canned tool results, and the live model the only thing on the
network. Setup and how to read a failure are in
[eval_setup.md](eval_setup.md).

What it checks is shape, never prose: which tools were called, with what
arguments, how many times; whether `[wait]` armed; whether the turn went
`[silent]`; which memory row and which cron job landed, and when the job
fires against a clock frozen to the fixture's `now`; whether every
`$` figure in the reply came from a tool result or the human's own words;
whether every `aim_log` line does too. A fixture can seed a ledger and
memory rows with an age, and its expects can name a seeded row by
position (`{{memory:0}}`, `{{ledger:1}}`) so "forget *that* id, not a
query" is checkable.
A call the agent blocks (prefix off, never enabled) counts as never made,
which is what the host would have seen. Every fixture runs N times and
every run must pass, because a rule that holds two times in three is a
rule the persona is not carrying. It runs on demand and as the job in
front of GoReleaser on a tag; never on a pull request, because it costs
money and forks do not get the secret.

Five kinds of fixture. **Single-batch rules**: one tool round and a
reply — the scoop reminder, the empty-day nudge, the off-prefix enable,
the "thanks, sounds good" that must not end in a bare acknowledgement.
**Completion**: the legwork happened — the flight search *called* this
turn from the human's own city with the options in the reply, Garmin
*read* before any progress answer, the planner nudge tied to the dinner
actually on the calendar — and the model may take the rounds it needs.
**Spend**: a metered API driven the way its own description asks — one
`listings_search` with the neighborhoods comma-joined, no per-listing
detail call nobody asked for, at most two flight searches when "next
Friday" is honestly two dates, no `dates_search` for a fixed date, no
checkout lookup before the human picks. **Ladder**: a seeded ledger
puts the planner on a rung — a streak with no `praised` this week, a
slip already `nudged`, a weight on pace — and the fixture checks the
planner took the next rung and not the last one, logged yesterday from
the tool it called, and scored a night out once against three aims;
`19_chat_rescore` checks that "that dinner was planned" rewrites the
row by `event=` instead of adding one. **List**: a stated errand
becomes one `todo/` row in the human's words with no cron and no
question; "make that Thursday" rewrites the same subject; "booked it"
forgets the `#id` on the stamp and never the query (the dentist's phone
number is seeded to catch that); a Wednesday planner schedules the cue
for the task whose words say `Wed 11am` and nothing for the one that
says Friday; a nine-day-old task gets a line and not an offer to drop it.

Where a fixture names a server, the tool defs are **real**. `tools_from`
pulls that server's latest GitHub release at run time with the same code
as `gantry tools-fetch`, boots it, and takes `tools/list` — names,
descriptions, schemas — as the catalog. Nothing is checked in and nothing
is called; the manifest behind it is name, binary, release URL, and a
placeholder env for binaries that refuse to start without a key present.
A renamed or dropped tool fails the fixture before any model call. A
fixture that proves the model can drive the harness author's idea of a
tool proves nothing about the binary; these prove the binary.

The round count is a note, not a gate. Each fixture carries a
`round_budget` — the rounds its rule needs — and a run over it is printed
beside the pass and counted at the end, never failed. The product's rule
is the opposite of the token-saving reflex that makes a cheap model not
worth using: the agent may spend ten rounds if the task takes ten, and
the failure the eval exists to catch is stopping at two with "let me know
when you want me to look." What the budget does catch is rounds in which
the model did nothing new — the same tools called again, the same rows
stored again — and those trace to a sentence in the seed or a rule in the
kernel, not to the model. Current readings on `gemini-3.6-flash`: the
original suite about 2.2 rounds and 13k prompt tokens per turn, with the
off-prefix fixture at a steady three because the enabled schema arrives
on the next call; the ladder planners about 3.1 rounds and 23k, because
a planner with `[progress]` on board is the fattest turn the harness
makes; the list fixtures 2.0 rounds and 12k on chat and 2.3–3.0 and
19–25k on the planners. The tasks eval is a clean example of what the
gate is for: the first pass had every planner green and every chat turn
red, and all three misses traced to two sentences in the `memory_store`
description (`todo/` sat next to `follow/` with nothing to tell them
apart; nothing said the `#id` on the stamp made a lookup unnecessary).
Three passes of description edits, no expect loosened except one `\?`
that was stricter than the documented contract, and the chat turns
went to two rounds and one tool call each
([tasks.md](tasks.md#what-the-eval-taught)).

Its limits. One model is one reading — the gate is on whatever is in
`.env`, and a Gemma-class local model would need its own pass and
probably its own tolerances. The client has no temperature knob, so
variance is the provider's default — which is also what production gets.
Canned results are the same for every call to a tool, so two searches for
two dates come back identical; the shape gates do not depend on that, but
the reply's prose is thinner than it would be live. One scenario row has
no fixture (a landed joke → `self_note`) because it needs an expectation
that reads `SELF.md`. And a fixture is one morning: the ladder's promise
is "never the same line twice" across a week, and the list's promise is
"a different line from yesterday" for as long as the task sits there;
each is checked as one planner turn against one seeded state, which
proves the rung, not the week. `planner_week_start` also went 2/3 on the
last n=3 reading (no `aim_log` on one run), which is either variance or
a sentence to find.

---

## Where gantry is behind

Ranked by how much a user would feel it.

### 1. No authorization layer

Manifest membership is the grant; an allowlisted phone with tools mounted
*is* the operator. "Ask first before writing events / no email, spend, or
posts" is prompt text. OpenClaw's pitch is "trusted gateway, untrusted
execution, deterministic policy"; Hermes ships approval and authorization
for tools. Gantry has cost controls (result truncation, iteration cap,
per-server call budgets), not permission controls. The smallest step that
would close it is an `ask_first` list in `mcp.toml` whose tools return a
harness-side `[confirm]` prompt on the first call and run only on the
human's next turn; it fits the existing shape, since `guardEnable`
already intercepts every call before the host sees it. It is declined all
the same — [why](#tool-call-confirmation-ask_first) — so this is a known
cost, not a plan.

### 2. Harness stamp cost and template coverage

The volatile block is twelve possible tags — two more than at the first
reading, `[todo]` and `[progress]`. On the minimal golden turn (no
memory, one-line user message) the re-evaluated remainder — harness block
plus the user's words — estimates at 221 tokens without GPS and 245 with
(`volatile_est_tokens` in the payload test log); a full board adds hours,
aims with rating suffixes, todo with ids, loops, wakes, surface, room,
and last contact on top, and the planner adds a `[progress]` grid per
aim that is the largest single thing the stamp has ever carried (the
Sunday week lines more so). That is cheap next to a tool round, and every
tag saves at least one, but it is a number to watch as tags accrue — the
block should stay boring and bounded, and the full-board golden does not
yet carry `volatile_est_tokens`, so its growth is a feeling rather than a
number. The caps are the defence: five aims, five tasks, five loops, five
days of grid, each with an explicit `(+N more — …)` instead of a silent
cut. Separately, `LLM_SYSTEM_FOLD=many`
(default for non-Gemini) is unverified against local chat templates that
render system only at position 0 (Gemma). If a local model cannot say NOW
without a tool, that is the first thing to check
([todo.md](todo.md#harness-stamp-harness-block)).

### 3. The behavior gate is one model and paid

The eval reads one model — whatever `.env` names — and a release gate
that costs money will not run on every push. Both are the price of a
contract on behavior rather than bytes, and both are accepted; but a
seed change proven on Flash is unproven on a local Gemma until someone
pays for that reading too.

### Smaller seams

- Location pin is in-memory; restart forgets it.
- Token counts are estimates unless the provider sends `usage`.
- Web search is Brave only.
- Discord and Slack lack the Telegram polish (menu, photos, reactions,
  pin, error tee).
- Consolidator is the chat model in a batch of 20; it can quarantine a
  batch and is not a second "memory model".
- `skill/<area>` rows exist only if the model stores them; a bland model
  on a fiddly tool will re-learn the pitfall. That is the auto-save-off
  trade, applied to procedure.
- A score is the agent's opinion. Two models, or one model on two days,
  can score the same burger `-1` and `-2`; the rubric and anchors narrow
  that, the rating averages over it, and the human can re-score any row,
  but a 30-day mean of opinions is not a measurement. The kernel's
  measurement trend (`weight 191.4 lb`) is the meter; the score is the
  read.
- Tasks have no done history. `/todo done` and `memory_forget` are a
  hard `DELETE`; only a rewrite leaves a superseded row behind. "How
  many things did I clear this month" is a question nothing here can
  answer, by choice — the doc says no done ledger — but it is the first
  thing a human who likes crossing things off will ask for.
- The task's due date is its words. `renew, by Oct 15` is a sentence the
  planner reads each morning against the week grid, not a field; a
  model that misreads "Wed" costs one cue, and nothing kernel-side would
  notice.
- `[room]` rides `[harness]` on every pendant turn — theme, wallpaper,
  face, each with an age, and one clause: yours, redress when the hour or
  mood moves on. No recipe; the tool descriptions have it, and a how-to on
  the stamp is what overloads a small model. Both prerequisites gate it in
  code: the pendant channel is the only `RoomSource`, and the line is
  skipped unless `pendant__*` is in the catalog. After a crane restart the
  line reads `theme not seen since boot` until the next change, because
  the Worker flushes theme to phones on connect and not to the crane — a
  three-line Worker change tracked in
  [todo.md](todo.md#harness-stamp-harness-block). Whether one clause
  produces taste or churn is a model question the goldens cannot answer;
  `/toolstats` shows `pendant__theme_update` counts over a week.

---

## Declined on purpose

Comparators list these as features. Each was considered, and the reasoning
is worth keeping next to the decision.

### Tool-call confirmation (`ask_first`)

An `ask_first` list in `mcp.toml` — flagged tools return a harness-side
`[confirm]` and run on the human's next turn — would fit the shape.
Declined. It costs a turn on exactly the calls a personal assistant makes
most — write the event, send the invite — and the point of this harness
is fast and cheap. "Ask first" stays prompt text, in the persona and the
daily planner. This is a POC; an operator who points it at a board-level
inbox has made that call themselves.

### Skills files

A `SKILL.md` per workflow duplicates the MCP manifest — the tool
description already says what the tool is for and how to call it, and two
copies drift. The catalog is the source of truth; `mcp_enable` and the
`tools` / `exclude` filters are the disclosure knob; `skill/<area>` memory
rows carry the exceptions. Hermes's background review agent that writes
skills after every session is a second model call per session producing
files a human never approves; the Curator then spends more calls pruning
them.

### A synthesized user model, re-injected every turn

Letta's `human` block and Hermes's Honcho prefetch put a "who this person
is" block in every prompt; Honcho's dialectic synthesis is a model call
to produce it. That is spend on something the chat history already
carries. Gantry captures at compaction: the fold writes one `Facts:`
paragraph into `session.summary` (standing, cached, rewritten only when
history folds); `SELF.md` holds the 3–5 north-star sentences; per-turn
hydration is ≤30 typed rows selected by FTS on the user's own words, no
model call. Compaction and capture, yes. Re-deriving the person every
turn, no.

### Pre-compaction memory flush

OpenClaw runs a silent agentic turn before compaction: "store durable
memory now." It is a second Completer call per fold, and it stores what
the model, a moment earlier, did not think worth a `memory_store`. On a
small model that is a hedge against forgetting; on a frontier model it is
a flood of rows the model did not want, which then cost hydration tokens
and `memory_forget` calls to undo. Gantry's fold already makes exactly one
model call and its output is `Facts:` + `Voice:`. That is the flush. Rows
enter typed memory only by deliberate `memory_store`, or by the
consolidator promoting an `episode` that was itself stored deliberately
or parked on `/new`.

### One gateway across every messenger

OpenClaw, Hermes, and Letta run one process that holds Telegram, Discord,
Slack, WhatsApp, Signal (and for OpenClaw iMessage, Teams, Google Chat,
Matrix) credentials at once. That is the home-computer model: one
process, every secret, a Control UI on a port. Gantry's model is one
hardened Distroless host per mouth, allowlist per mouth, no port — a
compromised mouth is one container, and "many hosts" is compose, not a
dashboard. Phone and car are the pendant and cab mouths, which the
household owns end to end. Channel count is not the axis this is built on.

### Model routing and failover inside one process

OpenClaw routes per agent and fails over per route; Letta and Hermes
carry a provider matrix. Gantry is `LLM_BASE_URL` + `LLM_MODEL`: one
socket, one brain per unit. The platform is not one process fanning out
to many models; it is Gantree running a **fleet of units, one agent per
human**, each with its own `.env`, persona, and brain. Model variety
lives across units, where it belongs: one household on Gemini Flash, one
on a local Gemma, one on xAI, each a separate container with a separate
blast radius, and the fleet dashboard is where you see them side by side.
A router inside the unit would carry the per-model glue
(`thought_signature`, grammar retries, `LLM_SYSTEM_FOLD`) behind every
route, put every household's failover on one process's decision, and buy
nothing the fleet does not already give by choosing per unit. Same
argument as the gateway above: parallel small units, not a funnel.

### Cheaper rounds by planning less

Lower `reasoning_effort`, and a one-round "speculative reply" for
write-only turns, would both cut the per-turn bill. Declined for the same
reason the round budget is a note and not a gate: both are cheaper because
the model plans or confirms less, and the failure this harness is built
to avoid is the cheap model that stops early. Rounds in which the model
does nothing new are removed by fixing the sentence that caused them, not
by telling the model to think less.

### A typed goal target

A grade table, a shape enum (`increase` / `decrease` / `streak`), a
target value per aim, a plan generator. Every one of those is a curve
someone will argue with (V-scale against YDS, volume against intensity,
whether pizza is `-1` or `-3`), and the model already knows the
domains. So the aim stays one sentence, the agent scores each event
against a fixed seven-point rubric with per-domain anchor rows, and the
human argues in chat and re-scores by id. The kernel only does what a
kernel is good at: means, buckets, a slope, a Pearson on measurement vs
effort. Charts are the yard's. There is no second model call for
analytics.

### A task tracker

A `task` table with due dates, priorities, projects, a done ledger, and a
`task_add` / `task_complete` tool pair. Declined; the doc's own line is
"if the tasks list needs to be bigger and more complex they are using
the wrong tool." A task is a `fact` row under `todo/`, the due date is
whatever the human said in the words, the planner is the parser, and
the cue is a cron pinned to the row. Adding a row is `memory_store`,
changing it is the same subject again, closing it is `memory_forget` by
the id on the stamp. The phone's checkbox is the one path that skips
the model (`/todo done <id>`); its add field is plain text, because
naming the slug is a model job. Two things stay firm: nothing on the
list ages out (a task the human has not done is still a task), and the
agent never proposes dropping one — day nine offers to put an hour on
the calendar, not to forget it.

### Voice in the harness

OpenClaw and Hermes do voice in the agent. Priced and declined: harness
voice means paying an STT/TTS API per turn. The cab mouth uses the phone's
on-device recognizer, the harness sees text, `[surface] android_auto` (or
`[input] spoken` from a pendant hold-to-talk turn) asks for a few short
plain sentences with no markdown, and the phone reads it back. Same
experience, no bill.

---

## How memory gets written

Every path a fact, a preference, or a bit of personality can take into
durable storage. This is the whole answer to "auto-save is off": nothing
below writes a typed row on the harness's judgment alone.

| Path | Trigger | Who decides | Lands in | Model call |
| --- | --- | --- | --- | --- |
| `memory_store` | Model calls the tool mid-turn | Model, deliberately; tool text says "Never auto-save guesses" | `memory` row: `fact` / `preference` / `person` / `episode` / `insight`; same kind+subject supersedes the live row (old row kept). Aims are `insight` `aim/<area>`; tasks are `fact` `todo/<slug>` | The turn's own |
| `aim_log` | Model calls it on the planner turn, or when the human states or disputes something | Model; the eval's `evidence_from_turn` gate requires the `what` to trace to a tool result or the human's words | `aim_event` + one `aim_score` per aim touched; `ref` replaces, `event=` rewrites; superseded rows kept | The turn's own |
| `/aims block`, `/todo done` | Human types or taps | Human | `aim_block` row; `memory_forget` on the todo row | None |
| `self_note` | Model calls the tool when personality happens | Model; "not facts about the human" | `SELF.md` (cap ~4KB) | The turn's own |
| Fold summary | History exceeds bounds | Harness triggers; chat model writes | `session.summary` as `Facts:` paragraph + `Voice:` ledger — **not** typed rows | One, at fold |
| Voice graduate | Same fold | Harness diffs new `Voice:` bits | `SELF.md` | None |
| `/new` distill | Human resets | Chat model merges quoted jokes / nicknames / aims; bland sessions skip the rewrite | `SELF.md`; the `Facts:` paragraph parks as one `episode` row | One, on `/new` |
| Consolidator | Timer, default 30 min | Chat model extracts durable rows from unconsolidated `episode`s (batch 20; quarantine after 3 bad JSON) | `fact` / `preference` / `person` / `insight` rows, `source=consolidation` | One per batch |
| GPS pin | Pendant frame or Telegram pin | Harness | In-memory only; restart forgets | None |
| `mcp_enable` | Model or `/short` / `/brief` | Model or human | `mcp_enable` rows with TTL (27h / 6h), touched on use | None |

What "auto-save on" would mean, and why each variant is off:

- **Harness stores every turn as an `episode`.** Consolidator then has to
  sift chit-chat for facts; hallucinated emails and misheard names become
  `fact` rows with `confidence 1.0`. Rejected in [design.md](design.md#memory-design).
- **Silent flush turn before fold** (OpenClaw). Second model call per
  fold; stores what the model just declined to store. Declined above.
- **Periodic "persist what you learned" nudges** (Hermes). A standing
  instruction that costs tokens every turn and produces Markdown a human
  does not review. The examples seed does this once, on demand, with a
  concrete recipe.

What gantry does instead: the tool descriptions carry the policy ("Facts
about the human go here — not `self_note`"; "Months-scale plans:
`kind=insight, subject=aim/<area>`"; hours as `pref/hours`; "a thing
THEY have to do: `fact subject=todo/<slug>`, not `follow/`"), the
`[harness]` stamp shows the model what is already stored (`[hours]`,
`[aims]`, `[todo]`, `[loops]`) so it does not re-store or re-query,
`memory_forget` makes every row correctable, `aim_log event=` makes
every score arguable, and `sqlite3` makes every row visible. The known cost is
in [features.md](features.md#the-okay): "Models forget to `memory_store`.
You will re-teach facts." That is the trade, and it is the right one for
a system where the persona file outranks recall and the operator owns the
veto.

---

## What not to trade away

Things the comparison could tempt someone to "fix" that are load-bearing:

- **No inbound port, no UI.** Every comparator has a control plane that
  listens. Gantry's health is an exit code and its console is the chat.
  That is why it fits a Distroless image and a small box, and why the
  yard (gantree) can exist as a separate product.
- **Manifest is the recipe.** Keep procedure in tool descriptions and the
  `skill/<area>` exception rows. Do not grow a skills directory.
- **One model call per fold.** The fold is the flush. Do not add a second.
- **Auto-save off.** Rows enter typed memory by deliberate `memory_store`
  or by the consolidator promoting a deliberately stored episode. Do not
  flip the default; see the table above for why each variant loses.
- **Persona outranks memory.** Recalled facts never override the operator
  file. Keep it.
- **Facts in SQLite, personality in a file.** The split is what lets an
  operator prune jokes with a text editor and audit facts with `sqlite3`.
  Comparators put both in Markdown; that is easier to read and harder to
  query.
- **A batch is read back whole.** Tool collapse counts rounds, not
  payloads. The persona asks for parallel calls; hiding one answer of a
  three-call batch makes the model ask again on a paid API or fill the
  hole with a number no tool returned. Keep the unit the round.
- **Goldens as the contract, fixtures as the behavior.** A stamp change
  regenerates and re-reads the goldens; a seed change re-runs the eval.
  Keep `-update` a deliberate act and the eval a gate on tags.
- **The event is the key; the agent scores, the kernel counts.** One
  night out is one row scored against every aim it touches. Do not add a
  per-aim target curve, a grade table, or an analytics model call. The
  evidence gate is what keeps the ledger honest; keep it.
- **Only the human closes a task, and nothing on the list expires.** The
  stale cue that `[loops]` carries is right for `follow/` and wrong for
  `todo/`. Do not add a falloff, a "still want this?" rung, or an
  auto-done from a calendar event.
- **A fixture miss is a sentence to fix, not an expect to loosen.** The
  tasks eval went red-to-green on description edits alone. The one time
  an expect moved, it moved to match the documented contract, and the
  doc says so.

---

## What is left

Three things of substance. `LLM_SYSTEM_FOLD=many` is unverified on a
Gemma-style local template. The full-board golden does not carry
`volatile_est_tokens`, and the board has grown by `[todo]` and a planner
`[progress]` grid since that was first written, so stamp growth is still
a feeling rather than a number. And the two behaviors the ledger and the
list promise across days — a different line each morning, a rung climbed
and not repeated — are each proven as one morning; a multi-day fixture
(seed a ledger, run the planner, seed its note, run tomorrow's) is the
missing eval shape. Everything else above is declined with its reason
attached or a seam small enough to live in [todo.md](todo.md): the
`self_note` fixture, the Worker's theme flush to the crane, the in-memory
location pin, the `planner_week_start` 2/3. Channels, routers, subagents, skills files, flush turns,
and UI stay out; they are the other products' shape, not this one's.
