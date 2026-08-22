# Features — the inventory

The [root readme](../readme.md) is the pitch. This page is everything we
actually built, ranked by how proud we are of it — not by how loudly it
shows up in a hello path.

This repo is the **AI harness** (`gantry`): one static Go binary — persona +
one OpenAI-compat model + optional MCP children + SQLite. The goal is
**long-horizon planning**: chat, memory, cron, watches, personality that
survives `/new` — **zero MCP required**.

A full life-stack (Workspace / Strava / Garmin / …) is **not this tree**.
One-off consumers: [`examples/`](../examples/). N isolated agents on a box
is a sibling console — [gantree](https://github.com/shotah/gantree) — not an
appliance folder in this checkout.

---

## The Great

The stuff that is the product — the harness work that makes a long-horizon
agent, not a chatbot. Other stacks skip these, or do them as a SaaS feature
you cannot inspect.

### Personality that survives `/new`

Most agents *feel* like someone after a long chat, then a reset lobotomizes
them. Gantry keeps the growth on purpose.

| Piece | What it does |
| --- | --- |
| **`SELF.md`** | Agent-writable notes in `PERSONA_DIR` — voice, jokes, rituals, a few north-star aims (not progress logs). The harness stamps the header (and the `PERSONA.md` Self-notes + Location pins sections) on boot / SIGHUP. Cap ~4KB. |
| **`self_note`** | Builtin tool: append one short line when personality happens mid-chat. Skip if it is already in the file. Not for facts about the human. |
| **Voice graduate on trim** | When history folds, new `Voice:` bits append to `SELF.md` **without another Completer call**. Mood weather (`dry today`) is skipped. Already-listed bits are skipped. |
| **Distill on `/new`** | Before the wipe: merge quoted jokes, nicknames, games, north-star aims into `SELF.md`. Bland tool sessions (no Voice, no quoted bits, short history) **do not rewrite the file**. |
| **You own the veto** | Delete lines. Wipe the file. Mount persona `:ro` to disable. Treat it like a friend’s inside jokes — keep what’s good, cut what isn’t. |

`PERSONA.md` is operator-owned (kernel only stamps Self-notes / Location pins).
Facts about you park in SQLite, not in the persona file. How to write one:
[persona.md](persona.md). Drift:
[troubleshooting.md](troubleshooting.md#selfmd--personality-drift).

### Voice vs Facts (token tuning that keeps the person)

History cannot grow forever. The fold is the whole game:

- Dropped turns become a rolling `session.summary`: **`Facts:`** (one tight
  paragraph) + **`Voice:`** (8–12 short lines, up to 8 verbatim quotes).
- Voice **copies forward unchanged** unless a new joke, nickname, or game
  appeared. A paraphrased joke is a dead joke — the prompt says so.
- On `/new`, Voice distills into `SELF.md`; Facts park as a memory episode.
- **Filler strip** (`HISTORY_STRIP_FILLERS`, default on): older **user**
  turns drop a small Go word list (`the`/`a`/`is`/… plus a few hedges). Last
  40 messages stay verbatim. **Assistant turns are never stripped** — they
  are the few-shot register. Quoted spans stay intact. SQLite is not rewritten.

That is the “keep personality while culling tokens” work. `/tokens` shows
whether Voice is present in the standing prompt.

### Small-model tool loop

Frontier stacks assume tool calling just works. 4–30B local models misspell
names, park answers in chain-of-thought, or print a call as JSON. The loop
repairs those instead of erroring the turn.

| Repair | Behavior |
| --- | --- |
| **Prefix alias** | `google_search__…` → `google-search__…` (hyphen/underscore in the *server* prefix only) |
| **Closest-name hint** | Unknown name returns the nearest real tools, model-facing |
| **Grammar-constrained retry** | Next Completer call is constrained to those candidate names (`response_format` json_schema). Ollama often omits `tool_calls` under a grammar — we parse the call out of the text and still run it |
| **Printed-call salvage** | Model writes `{"name":…}` as the reply → execute it, don’t show JSON to the human |
| **Think stall promote** | After tools, CoT-only output is promoted to the reply instead of another ERROR round |
| **Landing call** | At `TOOL_MAX_ITERATIONS` a final **no-tools** call forces a real reply (warning at ~70% of the budget) |
| **Gemini 3 signatures** | `thought_signature` on tool rounds is preserved (and synthesized when stream deltas omit it) so multi-step cloud turns don’t 400 |

Same levers cut prompt tokens on Flash/ChatGPT: schemas, history, and tool
results are re-billed **every** turn.

### Parallel tool calling

One model message can emit a **batch**. Independent calls in that batch run
concurrently; results are appended in the original `tool_call_id` order.
Same-server stdio still serializes inside the MCP host (one child, one pipe).
Long chains show motion in the bubble (`TOOL_TRACE`).

The standing prompt is **re-billed every Completer round**. A “cheap” local
loop — 20 rounds × 3 serial tools, recovery nudges — costs more tokens and
wall time than a fatter batch: 6 rounds × 12 parallel calls, clean
trajectory. Win the trajectory, not the individual call. Persona + the
kernel narration note teach: independent lookups in one response; chain only
when a later call needs an earlier result. `/perf` shows invocations, tools,
max batch, recoveries, prompt/gen estimates so you can see whether a change
did more work per decision.

### Quiet watches

A watch is a **cursor + poll**, not a chat loop. Quiet ticks call an MCP
fetch tool and **never** touch the Completer. New item ids wake the agent;
`[silent]` skips the push. First poll seeds the cursor (no backlog dump).

Do not fake this with cron + “fetch the feed.” That would bill a model call
every tick. [watch.md](watch.md).

### Chat is the console (including login)

No dashboard, no inbound port. Ops and OAuth live in the same allowlisted
chat.

- Slash commands: `/status` `/perf` `/memstats` `/toolstats` `/tokens`
  `/tools` `/examples` `/new` `/cancel` `/auth` `/help`
- Telegram refreshes the `/` menu on every bot start (`setMyCommands`)
- **`/auth`**: headless MCP OAuth — PKCE paste, device flow, or MFA code.
  Static catch page on GitHub Pages; no laptop `localhost` callback required.
  [auth.md](auth.md)

### Outbound-only + inspectable memory

Nothing listens. Health is `gantry status` (exit code) reading a SQLite
heartbeat. Memory is typed SQLite + FTS5 you can `sqlite3` — no embedding
API, no vector SaaS. Auto-save is **off** (hallucinated emails are worse than
forgetting). Persona files outrank recall. [memory.md](memory.md).

---

## The Good

Shipped, used in production, not the reason the repo exists — but you would
miss them.

### Agent loop hygiene

- Eager tool schemas at boot; refresh on child restart
- Each tool result truncated (`TOOL_RESULT_MAX_CHARS`)
- Tool payloads older than the last 2 collapse to one-line markers; matching
  tool-call args are stubbed. Session history never stores fat payloads
- `thought_signature` kept even when args are stubbed
- Per-objective logs: `iterations` / `tool_calls` / `max_batch` / `recoveries` /
  `prompt_est_tokens` / `gen_est_tokens` / `model_ms` / `tool_ms` / `total_ms`
- Fail-fast env: missing required vars = clear error + exit 1

### Streaming UX

- `STREAM_REPLIES` default on — Telegram edit-in-place / stdio token stream
- `SHOW_THINKING` — CoT live italics → expandable blockquote
- `TOOL_TRACE` — `compact` / `full` / `off`
- **Spin-up notice** (`SPINUP_NOTICE_MS`): local models prefill in silence;
  first turn after start posts “working” immediately
- **Multi-bubble steer** (`COALESCE_SETTLE_MS`): follow-ups join the live
  turn; Completer cancelled, in-flight MCP kept; Gemini signatures stay
- Telegram **reactions** settle into a `[reaction]` line (not a fake user
  message that steers the live turn)
- Photos: inbound → vision; outbound `SendPhoto` for image URLs in the reply
- Telegram location pin → `[last pin]` on the temporal footer (`internal/here`;
  in-memory, dies on restart)

### Time, cron, spark, examples

- Per-turn **temporal footer**: NOW, day-part, yesterday/today/tomorrow,
  week grid with ISO dates so “Monday” cannot reuse after the week rolls
- **Cron**: SQLite jobs, `cron_schedule` / `cron_list` / `cron_cancel`,
  timezone, overlap policy, `[silent]` skip-push
- Live-data cron: tool-first wrapper; zero-tool draft gets one nudge, then
  refuse invented metrics; prior `[cron]` turns omitted so yesterday’s
  digest cannot few-shot the next one
- **Spark of life** (`SPARK_QTY`, default `2-3`): random horizon wakes in a local
  hour window — replan against `SELF.md` / `aim/`, tool-call, `cron_schedule`;
  empty board asks once for a north-star; `[silent]` unless the human needs a
  message; skip if they messaged recently. `SPARK_QTY=0` turns it off.
- **Examples** (`EXAMPLES_QTY`, default `1-2`): capability pings from the
  live catalog (plus harness recipes: first aim, cron, memory) so you learn
  what the agent can do; `/examples on|off`; `/examples` on-demand still
  works when proactive is off

[cron.md](cron.md).

### MCP host

- Manifest **is** the grant (`mcp.toml`)
- `{server}__{tool}` names, optional `tools` / `exclude` / `tools_prefix`
- `--tool-tier` / preset args stay in the child (Garmin `core`, Google
  `everyday`)
- Fail-soft boot: one dead child logs `mcp server boot skipped`; the rest run
- Restart with backoff; stderr → slog
- `gantry tools-plan` / `gantry tools-fetch` pin GitHub release binaries
- **`mcp_enable`**: dynamic prefix publish for this chat (short ~27h / brief
  ~6h hold) so Flash is not fed a 150-tool catalog every turn
- `MCP_ENABLE_FORCE` always-on prefixes; `dynamic_tools = false` full-catalog
  rollback
- `TOOLS_ENABLED=false` for models that reject tools (e.g. Ollama gemma3)

[mcp.md](mcp.md) · [mcp-naming.md](mcp-naming.md).

### Channels, packaging, deploy

- Telegram (default, production), Discord Gateway, Slack Socket Mode, stdio REPL
- One `CHANNEL` per process; allowlist only; empty allowlist fails boot
- Distroless `static-debian12:nonroot`, `CGO_ENABLED=0`, linux/amd64+arm64
- Hub + GHCR (`:latest` / `:edge` / semver)
- Consumer templates: [examples/docker](../examples/docker/),
  [native](../examples/native/), [GCP](../examples/hosting/gcp/),
  [AWS](../examples/hosting/aws/)
- SIGHUP reloads persona; SIGTERM drains the in-flight turn then closes MCP
- JSON `slog` on stderr; optional **logfwd** tees WARN/ERROR into Telegram
  (deduped)

### Memory consolidator

Timer job (default 30 min) promotes `episode` rows → durable
`fact` / `preference` / `person` / `insight`. Bad JSON retries; quarantine
after 3 failures. Builtin backend only. Hydrate ≤ ~30 rows per turn.

---

## The Okay

Works. Has seams. Don’t be surprised.

| Feature | Why it’s only okay |
| --- | --- |
| **Token counts** | Chars/4 **estimates**, labeled as such. `/tokens` is a standing-prompt breakdown, not a tokenizer. Good enough to catch a fat schema; not a billing meter. |
| **Discord / Slack** | Shipped, outbound-only, allowlist. Telegram is the path that got the menu, photos, reactions, pin, error-tee, and production scars. |
| **Spark auto-bind** | Telegram DMs only. Other channels: schedule `repeat=spark` yourself. Default `2-3`/day; `SPARK_QTY=0` to disable. Work-only `[silent]` means most wakes never show up in chat — look at logs / `cron_list`. Empty board asks once. |
| **Examples pings** | On by default (`1-2`/day). Useful as training wheels; can feel like a nag. `/examples off` or `EXAMPLES_QTY=0`. |
| **`mcp_enable` holds** | Magic durations (27h / 6h). Wrong prefix → still a fat schema until idle expiry. |
| **Location pin** | In-memory. Restart = amnesia. Not a Completer wake. |
| **Memory auto-save** | Off on purpose. Models forget to `memory_store`. You will re-teach facts. |
| **Consolidator** | Same chat LLM, batch of 20, can quarantine a batch. Not a second “memory model.” |
| **Persona concat** | Two files (`PERSONA.md` then `SELF.md`). Extra `*.md` ignored. No DAG, no includes, no MCP catalog in persona. |
| **Fail-soft MCP** | A broken server does not take down the agent — and silently omits that whole capability until you read logs. |
| **Cron live-data refuse** | Stops invented Garmin numbers after one nudge. Still not a proof the pull was the *right* pull. |
| **Signal** | Planned sidecar (`signal-cli`). Not a Bot API. Not shipped. |
| **Hub vs root readme** | Two copies of the pitch (`docs/dockerhub.md` vs `readme.md`) because Hub caps ~25KB and hates SVG. |

---

## The Ugly

Footguns, debts, and constraints we chose and still wince at.

### Personality is a loaded gun

`SELF.md` reinforces itself. A snarky night becomes “who I am,” then distill
keeps it. Distill can still flatten a quoted joke into a vibe word if the
model is having a bland day — we restore dropped quotes, we cannot restore
taste. **You must prune.** `:ro` persona silently disables the whole feature
(easy to miss). There is no approval UI for new bullets.

### No second ACL

Manifest membership **is** the grant. An allowlisted phone with tools mounted
**is** the operator. Prompt injection + calendar send is in-scope. We truncate
results and cap iterations; that is cost control, not authorization.
[security.md](security.md).

### Outbound-only is a product wall

No pairing. No inbound webhooks. No WhatsApp / Teams / Messenger. Health is
not an HTTP probe. Debug in Distroless means no shell in the image — MCP
children must be static too. Ops is chat + `ssh` + `sqlite3`. That is the
point; it is also why this will never feel like a “platform.”

### One process = one brain

Want another model or persona? Another unit. No fallback chain, no router, no
team workspace. Scaling is compose services, not a dashboard.

### Gemini glue and Ollama glue

`thought_signature` is required for Gemini 3 multi-step tools and ignored by
everyone else. Grammar-constrained retries fight Ollama (it drops
`tool_calls` when `response_format` is set). Both are production scars, not
elegant abstractions.

### MCP children inherit the process env

Unless the manifest overrides `env`, every listed server can see what gantry
sees. Split containers if that bothers you. We did not build a secret
firewall between siblings.

### Context math is still a knife fight

Filler strip + Voice ledger + schema filters + result caps + collapse still
lose to an operator who publishes 80 Garmin tools. `TOOL_SCHEMA_MAX_TOKENS`
can hard-fail boot; most people will not set it. Small local models will pick
the wrong tool when the catalog is a novel.

### `/new` is not undo

Reset distills (maybe), parks Facts (maybe), then deletes the session. There
is no “restore that chat.” Memory rows stay; the funny thread is gone except
what landed in `SELF.md`.

### This checkout is not a yard

Strangers who want two bots, SSH deploy, and laptop OAuth will not find that
as a tree next to `cmd/gantry`. Hello is still Hub compose with **zero**
tools ([deploy-docker.md](deploy-docker.md)). N instances is a different
product: [gantree](https://github.com/shotah/gantree) (yard console). Keep
house keys out of *this* git.

---

## Cheat sheet (everything, one table)

| Area | Pieces | Where |
| --- | --- | --- |
| Category | AI harness; goal is long-horizon planning | [positioning](positioning.md) · [design](design.md#harness-and-long-horizon-planning) |
| Personality | `SELF.md`, `self_note`, Voice graduate, distill on `/new`, operator prune | [troubleshooting](troubleshooting.md#selfmd--personality-drift) |
| History / tokens | Caps, filler strip, `Facts:`/`Voice:` fold, tool collapse, `/tokens` | [design](design.md) |
| Tool loop | Parallel batch, alias, closest-name, grammar retry, salvage, CoT promote, landing call, signatures; `/perf` trajectory | [mcp](mcp.md) · [design](design.md#progress-per-invocation) |
| Memory | store / recall / forget, FTS5, consolidator, persona precedence, no auto-save | [memory](memory.md) |
| Time | Temporal footer, cron, spark, examples, `[silent]`, live-data nudge | [cron](cron.md) |
| Events | Watch cursor + poll, Completer only on new ids | [watch](watch.md) |
| MCP | Manifest grant, fetch/plan, filters, `mcp_enable`, fail-soft, Distroless children | [mcp](mcp.md) |
| Chat ops | Slash cmds, `/auth`, stream, thinking, tool trace, steer, spin-up, photos, reactions, pin | [auth](auth.md) · [observability](observability.md) |
| Channels | Telegram / Discord / Slack / stdio, allowlist, no ports | [discord](discord.md) · [slack](slack.md) |
| Persona files | `PERSONA.md` → `SELF.md`, harness stamps | [design](design.md) |
| Runtime | Static Go, Distroless, heartbeat, drain, SIGHUP, logfwd | [architecture](architecture.md) |
| Deploy | Hub compose, native systemd+Ollama, GCP/AWS templates | [deploy-docker](deploy-docker.md) · [deploy-native](deploy-native.md) |
| Yard | Console, metrics, grant tools, several agents | [gantree](https://github.com/shotah/gantree) |
| Won’t | Dashboard, pairing, inbound webhooks, WhatsApp/Teams, multi-agent router | [positioning](positioning.md) |

Contract (env table, loop bounds): [design.md](design.md).
Diagrams: [architecture.md](architecture.md).
Why we picked X: [choices.md](choices.md).
