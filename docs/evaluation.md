# Harness evaluation

*Authored by Claude Fable 5.1 (Anthropic), working in Cursor, 2026-09-14;
revised 2026-09-15 after the behavioral eval landed and first ran live.
The opinions below — including the verdict — are the model's, not the
maintainer's. The maintainer supplied corrections where the first draft
misread a design choice; those are noted in place.*

An honest read of `gantry` as an AI harness — what it is good at, where it
is thin, and how it sits next to the other self-hosted personal-agent
harnesses people actually run in 2026. Written from the tree, the test
suite, and the Completer goldens, not from the pitch. Inventory is
[features.md](features.md); the loop is
[architecture.md](architecture.md); open work is [todo.md](todo.md).

Comparators were checked against their own docs on 2026-09-14:
[OpenClaw](https://docs.openclaw.ai/), [Letta Code](https://github.com/letta-ai/letta-code)
(and the [March 2026 direction post](https://www.letta.com/blog/our-next-phase/)),
[Hermes Agent](https://hermes-agent.nousresearch.com/docs/). This is a
design comparison, not a benchmark. Nobody here ran the four side by side
on the same tasks.

A first draft of this page counted skills files, a per-turn user model, a
pre-compaction memory flush, and a multi-channel gateway as gaps. They are
not. Each was tried or priced and declined; the reasons are in
[Declined on purpose](#declined-on-purpose) so the next reader does not
re-propose them.

---

## Overall impression

Gantry is a **disciplined, single-purpose harness**: one static Go binary,
one persona, one OpenAI-compat socket, optional MCP children, SQLite. It
refuses most of what the category is currently adding (UIs, routers,
subagents, skills marketplaces, inbound ports) and spends that budget on
five things the others mostly leave to the model — or do not do at all:

1. **What the model actually sees.** The prompt is a pinned contract. PWA
   inbound JSON → `Handle` → Completer request → HTTP body are all
   goldens, and the wire golden is the *same file* the agent-side test
   pins. When the harness stamps time, GPS, hours, aims, loops, wakes,
   surface, and last contact, the test suite proves the bytes land — on
   OpenAI-layout *and* on Gemini's one-system-instruction layout.
2. **Small-model tool hygiene.** Prefix alias, closest-name candidates,
   grammar-constrained retry, printed-call salvage, CoT promotion, landing
   call, thought-signature plumbing, result collapse. This is the
   unglamorous layer that makes a 4–30B local model usable as an agent
   instead of a demo — and `/toolstats` counts every repair.
3. **The tool catalog as the procedure.** Intent-first tool descriptions
   are the recipe; `[mcp prefixes]` publishes the catalog by name and
   ships schemas only for prefixes the chat enabled; fiddly exceptions
   live as `skill/<area>` memory rows recalled on demand. No skills
   directory, because it duplicated the manifest.
4. **Personality that survives reset, with an operator veto.** `SELF.md`,
   Voice ledger on fold, distill on `/new`, and a plain text file you can
   prune. Facts go to SQLite, not into the persona.
5. **A mouth the model can dress.** On the pendant and cab the agent
   owns its own face, the chat wallpaper, and the room's color mood —
   `pendant__avatar_update` / `backdrop_update` / `theme_update` from a
   picture it generated and a closed theme catalog it listed. The
   household owns the whole path (crane → Worker → every socket). No
   comparator gives the agent control of the client it is talked to in.

The cost of that discipline: no authorization layer beyond the allowlist
and the manifest, one model per process, a harness stamp that is now
eight tags and needs watching, and a behavior gate that costs money to
run — so it gates releases, not pull requests. Those are the gaps below;
the behavioral one was open when this was first written and is closed
now. Channel breadth, skills files, a synthesized user model, memory
auto-save, and voice are *not* gaps; they are declined, and the reasons
hold.

**Verdict (the model's, per the byline):** best-in-class at *harness-side
context*, at running well on weak models, and at keeping the tool catalog
cheap among the comparators read above; deliberately behind on
breadth (channels, UI, multi-agent). Right choice for one person, one
mouth, one brain on a hardened small box, with a phone and a car screen
the agent can make its own. Wrong choice for a team, a shared gateway, or
anyone who wants the agent to author its own tooling.

---

## Scorecard

| Area | Verdict | Why |
| --- | --- | --- |
| Prompt assembly & visibility | **Strong** | Byte-pinned goldens from mouth JSON to HTTP body. `[harness]` stamp names only present tags. Gemini fold order tested. |
| Time / place grounding | **Strong** | NOW, day-part, week grid with ISO dates, GPS with fix age, hours, next wakes — no tool call needed. |
| Small-model tool loop | **Strong** | Repairs instead of erroring; ≤5 closest-name candidates then a constrained retry; parallel batch; `/perf` shows trajectory shape; `/toolstats` shows repair counts. |
| Tool catalog cost | **Strong** | `[mcp prefixes]` is byte-stable (cacheable); `mcp_enable` TTL holds (27h / 6h); `tools` / `exclude` / `tools_prefix` filters; results collapse after the last 2; same-name repeats collapse inside the window. |
| Procedural memory | **Good, different shape** | Manifest + descriptions are the recipe; `skill/<area>` rows for exceptions; examples seed teaches the pattern. Depends on the model choosing to store — consistent with auto-save off. |
| Personality persistence | **Strong** | `SELF.md` + Voice fold + distill; operator prune; `:ro` kill switch. |
| Proactivity | **Good** | Spark (3–5/day, sleep-aware, `[silent]`), examples, cron, quiet watches. Spark reads `[aims]` / `[wakes]` instead of re-fetching. |
| Long-term memory (facts) | **Good** | Typed SQLite + FTS5, inspectable with `sqlite3`, persona precedence, consolidator. No embeddings by choice. |
| User model | **Deliberate** | Captured at fold (`Facts:`), not re-derived per turn. Per-turn cost is ≤30 FTS rows keyed on the user's words, no model call. |
| Memory capture | **Deliberate** | Explicit `memory_store` only; the fold is the one compaction call. No flush turn, no auto-save. See [How memory gets written](#how-memory-gets-written). |
| Channels | **Different model** | One hardened host per mouth, not one gateway holding every messenger credential. Telegram (production), Discord, Slack, pendant, stdio. |
| Model-controlled client | **Unique** | Face, wallpaper, and theme on the pendant / cab are the agent's to set (`pendant-mcp`, 7 tools). Picture handoff by `source_path`; theme from a closed catalog with mood lines; humans can unfollow. `[room]` on `[harness]` shows the current look with ages so the model — and spark — redress it without being asked. |
| Security / authorization | **Thin** | Allowlist + manifest-is-grant. No per-tool approval; ask-first is prompt text. |
| Ops surface | **Strong for one box** | No inbound port, Distroless, `gantry status` heartbeat, chat is the console, `/auth` headless OAuth. Fleet ops is gantree, not here. |
| Multi-model / multi-agent | **Absent by design** | No router, no fallback, no subagents. One process = one brain. |
| Behavioral regression | **Good** | Seven scenario fixtures replayed against the shipped seed on the live model; shape asserted (tools, args, `[wait]`, `[silent]`, rows, jobs), every run must pass. Release gate and on demand, not per push — it is paid. Three table rows still unfixtured. |
| Token accounting | **Okay** | chars/4 estimates plus native `usage` when sent. `/tokens` catches fat schemas, not billing. |

---

## Where gantry is ahead

| Dimension | Gantry | OpenClaw | Letta Code | Hermes Agent |
| --- | --- | --- | --- | --- |
| Runtime | One static Go binary, `CGO_ENABLED=0`, Distroless, no inbound port | Node 22+ Gateway with Control UI, binds a port | Harness + agents stored in Letta Cloud for multi-computer; local server option | Python agent + gateway process, seven terminal backends |
| Prompt contract | Goldens from mouth JSON to HTTP body; wire == agent layout by test | Not published as a pinned artifact | Not published as a pinned artifact | Not published as a pinned artifact |
| Harness-side context (no tool call) | `[current time]` week grid, `[location]` with fix age, `[hours]`, `[aims]`, `[loops]`, `[wakes]`, `[surface]`, `[room]`, `[last contact]` | Model reads workspace files; cron in gateway | Memory blocks in context (persona / human / custom) | `MEMORY.md` / `USER.md` loaded at session start; skills by name |
| Catalog disclosure | `[mcp prefixes]` on/off by server prefix, byte-stable; `mcp_enable` ships schemas next call with a TTL; force list for always-on | Full toolset per agent; skills by name | Tools + skills; skill text loaded on demand | Skill *name* in prompt, `skill_view` loads the file; 60+ builtin tools always on |
| Procedure lives in | Tool descriptions (manifest is the grant and the recipe) + `skill/<area>` memory rows | Workspace Markdown, skills | Skills + memory blocks, MemFS | `SKILL.md` files written by a background review agent, curated by a Curator |
| Weak-model tool repair | Alias, ≤5 closest names, grammar-constrained retry, salvage, CoT promote, landing call; counted in `/toolstats` | Assumes capable model ("use the strongest latest-generation model") | Model-agnostic, frontier-oriented | Model-agnostic; RL / trajectory tooling for training tool-callers |
| In-turn context bounding | Last 2 tool payloads in full; older and same-name repeats → one-line marker, args stubbed, signatures kept | Compaction at threshold | Compaction / MemFS | Compaction |
| Personality across reset | `SELF.md` Voice ledger, distill, operator prune | `IDENTITY.md` static; memory flush before compaction | Persona memory block, agent rewrites it | `SOUL.md` static voice; skills carry procedure |
| Agent controls its client | Pendant + cab: own face (`avatar_update`), chat wallpaper (`backdrop_update`), room mood from a closed theme catalog (`theme_list` → `theme_update`); Durable Object broadcasts to every socket | Canvas on companion apps — the agent renders *content* on a surface; client identity and theme are the user's | Chat at chat.letta.com / desktop app; no agent control of the client | TUI + messaging; no agent control of the client |
| Event watches | Cursor + poll on an MCP fetch tool; Completer only on new ids | Cron / webhooks | — | Cron |
| Memory inspectability | `sqlite3 gantry.db`, typed rows, no vector SaaS | Markdown files + SQLite index | Memory blocks; MemFS git-backed context repo | Markdown + FTS5 session DB + external Honcho |

Four of these matter most. **The prompt contract** — nobody else ships a
test that diffs the socket bytes against the agent layout; it is why the
time/GPS blind spot was found and closed in a day. **The harness stamp** —
time, place, hours, horizon delivered without a tool round. **Catalog
disclosure by prefix** — Hermes discloses skill names and loads text on
demand; gantry does the same thing one level down, at the tool schema,
which is where the tokens actually are, and the on/off line is stable
enough for prefix caching. **The agent's own client** — the others meet
the user in someone else's app (Telegram, WhatsApp, a TUI) or a vendor
web chat, so the agent has a name and a text bubble. On the pendant and
cab the agent has a face it drew, a wallpaper it picked for the hour, and
a color mood it chose to match how the day is going — and when it changes
one, every open phone and car screen repaints. The plumbing is what makes
it hold up: the picture never crosses the model as bytes (`source_path`
from `image__photo_generate`, encoded to budget in the MCP), the theme is
an id from a catalog with a mood line (no invented hex), the wallpaper
has a `delete`, and a human can unfollow and keep their own theme. It is
the connection feature. POC 1 proved the path; the `[room]` stamp is what
makes it a habit rather than a party trick — see the seam below for what
it still leaves on the table.

Why the skills comparison flips: Hermes and Letta need recipe files
because their tool surface is generic (bash, browser, a fixed builtin
set) — the procedure has nowhere else to live. Gantry's tool surface is a
typed MCP catalog whose descriptions lead with intent and document hot
paths ([mcp-naming.md](mcp-naming.md)). A skill that says "call
`google__calendar_list_events` with `time_min`" *is* the schema, so a
skills directory was a second copy of the manifest that could drift. It
was removed for that reason. What remains is the exception path:
`skill/<area>` rows with exact names and one pitfall, recalled before
guessing — the examples seed teaches it, and `mcp-naming.md` lists a skill
that cites a non-host tool name as an anti-pattern.

---

## Where gantry is behind

Ranked by how much a user would feel it. The first was the widest when
this was written and is closed; it stays here for the record and because
its limits are worth knowing.

### 1. Behavioral regression (closed)

The goldens are the best prompt-bytes contract in the category, and they
say nothing about what the model *did*. Until this week the only gate on
a prompt change was "the string contains `[aims]`" — while every spark
line and the persona seed itself were being rewritten.

What closed it is a second, paid contract beside the free one. Each row
of the scenario table in
[persona_doc_goals.md](persona_doc_goals.md#scenario-checks) is a fixture:
one turn — a human message or a real spark wake — against the **shipped
seed**, with the production store stack in a temp dir and canned MCP
tools, the live model the only thing on the network. The check is shape,
never prose: which tools were called and with what, whether `[wait]`
armed, whether the turn went `[silent]`, which memory row and which cron
job landed. A call the agent blocks (prefix off, never enabled) counts as
never made, which is what the host would have seen. Every fixture runs N
times and every run must pass, because a rule that holds two times in
three is a rule the persona is not carrying. It runs on demand and as
the job in front of GoReleaser on a tag; never on a pull request, because
it costs money and forks do not get the secret. The setup and how to read
a failure are in [eval_setup.md](eval_setup.md).

First live reading (`gemini-3.6-flash`): every run that ran, passed. The
scoop and the "thanks, sounds good" scenarios held ten for ten; the three
between them showed no failure in the windows seen before Go's default
ten-minute test timeout cut the run (the Makefile now sets its own); the
two that never got a turn — no aims, and the gym spark — then held three
for three. The shapes are the part worth reading. On the scoop the
model's own order every time was `cron_list → memory_store → calendar
event → cron_schedule` — check the board, store the follow, put the
sprint on the calendar, set the wake — when the fixture only demanded the
reminder. Off prefix went `memory_recall → mcp_enable → calendar` without
being told which came first. The spark called Garmin on every wake, in
either order with the calendar, and was never silent. "Hey" with no aim
on the board got one question, once with no tool at all. That is the
distilled seed carrying its rules, measured instead of felt.

Its limits. One model is one reading — the gate is on whatever is in
`.env`, and a Gemma-class local model would need its own pass and
probably its own tolerances. Three table rows have no fixture (a landed
joke → `self_note`; the weight/dinner spark; the Denver flight) because
they need an expectation that reads `SELF.md`, or canned search and
flights tools that do not exist. The client has no temperature knob, so
variance is the provider's default — which is also what production gets.
And the harness had one bug on its first outing: it opened `gantry.db`
twice and parallel tool batches fought over the write lock, a
`SQLITE_BUSY` the product never sees; it now shares the one handle like
`run.go`.

Goldens are the byte contract, free on every push; the eval is the
behavior contract, paid at release. Hermes and Letta export trajectories;
neither has the gate.

### 2. No authorization layer

Manifest membership is the grant; an allowlisted phone with tools mounted
*is* the operator. "Ask first before writing events / no email, spend, or
posts" is prompt text. OpenClaw's pitch is "trusted gateway, untrusted
execution, deterministic policy"; Hermes ships approval and authorization
for tools. Gantry has cost controls (result truncation, iteration cap),
not permission controls. The smallest step that would close it is an
`ask_first` list in `mcp.toml` whose tools return a harness-side
`[confirm]` prompt on the first call and run only on the human's next
turn; it fits the existing shape, since `guardEnable` already intercepts
every call before the host sees it. It is declined all the same —
[why](#tool-call-confirmation-ask_first) — so this stays a known cost, not
a plan.

### 3. Harness stamp cost and template coverage

The volatile block is now eight possible tags. On the minimal golden turn
(no memory, one-line user message) the re-evaluated remainder — harness
block plus the user's words — estimates at 221 tokens without GPS and 245
with (`volatile_est_tokens` in the payload test log); a full board adds
hours, aims, loops, wakes, surface, and last contact on top. That is
cheap next to a tool round, and every tag saved at least one, but it is a
number to watch as tags accrue — the block should stay boring and
bounded. Separately, `LLM_SYSTEM_FOLD=many` (default for non-Gemini) has
not been verified against local chat templates that render system only at
position 0 (Gemma). If a local model cannot say NOW without a tool, that is
the first thing to check ([todo.md](todo.md#harness-stamp-harness-block)).

### 4. One model, no fallback

OpenClaw does per-agent routing and failover. Gantry: `LLM_BASE_URL` +
`LLM_MODEL`, one socket, another brain is another unit. The Gemini and
Ollama glue (`thought_signature`, grammar retries, now `LLM_SYSTEM_FOLD`)
is per-model scar tissue that a router would have to carry per route. Not
a gap worth closing for one person; worth naming for anyone sizing this
up as a platform.

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
- The room tools used to be called only when asked. Now `[room]` rides
  `[harness]` on every pendant turn — theme, wallpaper, face, each with an
  age, and one clause: yours, redress when the hour or mood moves on. No
  recipe; the tool descriptions have it, and a how-to on the stamp is what
  overloads a small model. The spark note adds the same clause. Both
  prerequisites gate it in code, not prose: the pendant channel is
  the only `RoomSource`, and the line is skipped unless `pendant__*` is in
  the catalog (and says `mcp_enable pendant` when the prefix is off for
  the chat). What is left: after a crane restart the line reads `theme not
  seen since boot` until the next change, because the Worker flushes
  theme to phones on connect and not to the crane — a three-line Worker
  change tracked in [todo.md](todo.md#harness-stamp-harness-block). And
  whether one clause produces taste or churn is a model question the
  goldens cannot answer; watch `/toolstats` for `pendant__theme_update`
  counts over a week.

---

## Declined on purpose

Comparators list these as features. Each was considered, and the reasoning
is worth keeping next to the decision.

### Tool-call confirmation (`ask_first`)

Gap 2 above proposed an `ask_first` list in `mcp.toml`: flagged tools
return a harness-side `[confirm]` and run on the human's next turn.
Declined. It costs a turn on exactly the calls a personal assistant makes
most — write the event, send the invite — and the point of this harness
is fast and cheap. "Ask first" stays prompt text, in the persona and the
spark wake. This is a POC; an operator who points it at a board-level
inbox has made that call themselves.

### Skills files

Tried; removed. A `SKILL.md` per workflow duplicated the MCP manifest — the
tool description already says what the tool is for and how to call it,
and two copies drift. The catalog is the source of truth; `mcp_enable`
and the `tools` / `exclude` filters are the disclosure knob; `skill/<area>`
memory rows carry the exceptions. Hermes's background review agent that
writes skills after every session is a second model call per session
producing files a human never approves; the Curator then spends more
calls pruning them.

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
| `memory_store` | Model calls the tool mid-turn | Model, deliberately; tool text says "Never auto-save guesses" | `memory` row: `fact` / `preference` / `person` / `episode` / `insight`; same kind+subject supersedes the live row (old row kept) | The turn's own |
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
`kind=insight, subject=aim/<area>`"; hours as `pref/hours`), the
`[harness]` stamp shows the model what is already stored (`[hours]`,
`[aims]`, `[loops]`) so it does not re-store, `memory_forget` makes every
row correctable, and `sqlite3` makes every row visible. The known cost is
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
  `skill/<area>` exception rows. Do not grow a skills directory again.
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
- **Goldens as the contract.** Every stamp added this week regenerated
  and re-read five fixtures. Keep `-update` a deliberate act.

---

## What is left

One thing of substance: `LLM_SYSTEM_FOLD=many` has not been verified on a
Gemma-style local template, and the full-board golden does not carry
`volatile_est_tokens`, so stamp growth is a feeling rather than a number
(gap 3). Everything else above is closed, declined with its reason
attached, or a seam small enough to live in [todo.md](todo.md). Channels,
routers, subagents, skills files, flush turns, and UI stay out; they are
the other products' shape, not this one's.
