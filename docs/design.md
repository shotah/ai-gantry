# Design

Harness contract: principles, env, agent loop, memory, ops, packaging.
Pitch and hello path: [root readme](../readme.md). Diagrams:
[architecture.md](architecture.md). ICP: [positioning.md](positioning.md).

Gantry is an **AI harness** — the runtime around one model so an agent can
**plan on a long horizon**. The model predicts tokens. The harness makes a
turn finish, a tool call land, and a goal survive tomorrow.

## Problem

Platform agent stacks drift toward multi-agent products: multiple providers,
dashboards, console features, config UI. Our deployment model is the opposite:

```text
process = persona + model + MCP set + data dir
```

Want another LLM or persona? Another process (second compose service or
systemd unit). No in-process routing, no dashboard — a harness that does
exactly that and nothing else.

Deploy shapes: [deploy-docker.md](deploy-docker.md) (Hub) ·
[deploy-native.md](deploy-native.md).

## Principles

1. **Stupid simple.** One agent, one model, one channel loop. If a feature
   needs a diagram to explain, it probably belongs in an MCP binary, not here.
2. **Highly performant.** Pure Go, static binary, no CGO, small RSS, no
   background frameworks. Long-poll + goroutines; nothing dials in. Speed is a
   product feature: curated tool schemas, in-process FTS memory (no embedding
   round-trip), and a Gemini 3–compatible tool loop that preserves
   `thought_signature` so multi-step turns finish instead of 400’ing. See
   **Local-model hardening** below.
3. **Highly portable.** `CGO_ENABLED=0` static binary — systemd or Distroless
   (no shell in the image). No glibc dependency in our binary.
4. **Plugin-centric.** Capabilities come from external binaries over MCP
   stdio. The gantry **is the harness**: it hosts tools; it does not implement
   them (except a few builtins: memory, cron, watch, `self_note`). Import
   libraries over writing our own.
5. **1:1, always.** No multi-provider config, no multi-agent config, no peer
   routing. Scaling = more processes.
6. **Env + files is the config plane.** Secrets and scalars via env. Structure
   via persona markdown, MCP manifest, and a data directory.
7. **Memory is structured and inspectable.** SQLite rows you can read and
   delete with `sqlite3`, not opaque embedding blobs. Persona files always
   outrank recalled memory.
8. **Long-horizon.** The harness holds goals, personality, and work across
   sessions. Memory, cron, watches, history fold, and `SELF.md` are first-class
   — not extras you add when a chatbot gets boring.

## Harness and long-horizon planning

Industry language for what this binary always was.

An **AI harness** is everything around the model at runtime: the tool loop,
MCP host, context bounds, memory, persona, and the channel. The model is a
stateless token predictor. The harness is why a turn finishes, a name typo
still calls the right tool, and yesterday’s aim is still on the board after
`/new`.

**Long-horizon planning** is the goal: hold aims, personality, and work across
days and weeks — not a chatbot that dies when the context window gets
expensive. That is why these pieces live in the harness, not in an MCP:

| Horizon work | What the harness does |
| --- | --- |
| Standing goals | Cron + watches fire the same loop later; spark keeps presence |
| Personality | `SELF.md` / `self_note` / Voice distill outlive `/new` |
| Facts | SQLite memory + consolidator; persona files outrank recall |
| Context that does not rot | History caps, `Facts:`/`Voice:` fold, tool collapse |
| Turns that actually finish | Tool repair, landing call, local-model hardening |

A single chat turn is the unit of *execution*. The horizon is the unit of
*product*. We named it; we did not invent a second architecture.

## Non-goals

- Web dashboard, gateway, REST/WS API, pairing flows
- Multi-agent / multi-provider / model fallback chains
- Built-in search/workspace tools (those are MCP binaries)
- Vector DB / embedding service
- In-process sandboxing / risk profiles (the container is the sandbox;
  channel allowlist is the gate)

## Shipped milestones

M0–M7 are done (scaffold → talk → Telegram → MCP → memory → hardening → cron →
stream). Full checklist: [milestones.md](milestones.md). Cron:
[cron.md](cron.md). Watches: [watch.md](watch.md). Streaming:
`STREAM_REPLIES=true`.

## Local-model hardening

Most agent stacks assume frontier cloud models and huge tool catalogs. This
harness is hardened where 4–30B local models actually fail — and the same
levers cut prompt tokens on Flash/Grok (schemas, history, and tool results
are re-billed every turn). Long-horizon work is worthless if a mid-chain
tool turn 400s.

| Lever | What we do | Why it matters |
| --- | --- | --- |
| Tool surface | Manifest filters + MCP `--tool-tier` | Smaller schemas → better tool picks |
| Name repair | Prefix alias/rebuild, closest-name hints, then a grammar-constrained retry | `google_search__…` still lands |
| Think stalls | Promote CoT → reply after tools | Multi-step turns finish instead of ERROR |
| Printed calls | Parse a tool call written as text and run it | A model that prints `{"name":…}` never speaks JSON at you |
| Multi-bubble | Steer + settle (`COALESCE_SETTLE_MS`) | Follow-ups join the live turn; MCP calls kept |
| Memory | SQLite + FTS5 in-process | No embedding API before every reply |
| Personality | `SELF.md` + `self_note` + distill on `/new` | The funny agent survives resets |
| Runtime | One static binary (systemd *or* Distroless) | No Node/Bun/gateway in the path |
| Gemini 3 | Preserves `thought_signature` on tool rounds | Cloud multi-step turns don't 400 |

MCP tools share one `{server}__{tool}` name and one repair path. Details:
[mcp.md](mcp.md) · [deploy-native.md](deploy-native.md).

## Configuration contract

Everything is env or a mount. No config UI, no `config set`, no sync step.
Boot is fail-fast: missing required env = clear error + exit 1.

### Environment variables

| Var | Required | Example / default |
| --- | --- | --- |
| `LLM_BASE_URL` | yes | `https://generativelanguage.googleapis.com/v1beta/openai` |
| `LLM_API_KEY` | yes | — |
| `LLM_MODEL` | yes | `gemini-3.5-flash` |
| `LLM_MAX_TOKENS` | no | `4096` (completion output cap; `0` = provider default) |
| `LLM_REASONING_EFFORT` | no | empty (Ollama/Qwen: `none` disables thinking so max tokens aren't eaten by CoT) |
| `TELEGRAM_BOT_TOKEN` | yes (telegram) | — |
| `TELEGRAM_ALLOWED_USERS` | yes (telegram) | `123456789,987654321` (numeric IDs; **allowlist only — no pairing**) |
| `TELEGRAM_ERROR_REPORTING` | no | `off` (`off`\|`error`\|`warn` — tee slog into the Telegram chat) |
| `DISCORD_BOT_TOKEN` | yes (discord) | — |
| `DISCORD_ALLOWED_USERS` | yes (discord) | snowflake user IDs — [discord.md](discord.md) |
| `SLACK_BOT_TOKEN` | yes (slack) | `xoxb-…` bot token |
| `SLACK_APP_TOKEN` | yes (slack) | `xapp-…` app-level token — [slack.md](slack.md) |
| `SLACK_ALLOWED_USERS` | yes (slack) | Slack member IDs |
| `CHANNEL` | no | `telegram` (default), `discord`, `slack`, or `stdio` |
| `PERSONA_DIR` | no | `/persona` |
| `DATA_DIR` | no | `/data` |
| `MCP_MANIFEST` | no | `/etc/gantry/mcp.toml` |
| `HISTORY_MAX_MESSAGES` | no | `200` |
| `HISTORY_MAX_TOKENS` | no | `32000` (chars/4 estimate; older turns fold into `Facts:` / `Voice:`) |
| `HISTORY_STRIP_FILLERS` | no | `true` (prompt-only; last 40 messages verbatim; assistant never stripped) |
| `TOOL_RESULT_MAX_CHARS` | no | `6000` |
| `TOOL_MAX_ITERATIONS` | no | `10` (at the cap a final no-tools call forces a text reply) |
| `TOOL_SCHEMA_MAX_TOKENS` | no | `0` (log estimate only; `>0` = hard fail if over) |
| `TOOLS_ENABLED` | no | `true` (`false` omits all tool schemas — models that reject tools, e.g. Ollama gemma3) |
| `MCP_ENABLE_FORCE` | no | comma-separated prefixes always published when `dynamic_tools` is on |
| `SELF_NOTES_ENABLED` | no | `true` (auto-off when `PERSONA_DIR` is read-only) |
| `MEMORY_ENABLED` | no | `true` |
| `MEMORY_BACKEND` | no | `builtin` (or `mcp:<server-name>`) |
| `MEMORY_CONSOLIDATE_MINUTES` | no | `30` (`0` = off; builtin backend only) |
| `CRON_ENABLED` | no | `true` |
| `CRON_TZ` | no | `America/Los_Angeles` |
| `CRON_MAX_JOBS` | no | `50` |
| `CRON_TICK_SECONDS` | no | `15` |
| `WATCH_ENABLED` | no | `true` — [watch.md](watch.md) |
| `WATCH_MAX` | no | `50` |
| `SPARK_QTY` | no | empty = off (`5`, `4-6`) |
| `SPARK_START_HOUR` / `SPARK_END_HOUR` | no | `6` / `21` |
| `SPARK_PROMPT` | no | empty |
| `SPARK_SKIP_RECENT_MINUTES` | no | `30` |
| `EXAMPLES_QTY` | no | `1-2` (empty/`0` = no proactive pings) |
| `EXAMPLES_START_HOUR` / `EXAMPLES_END_HOUR` | no | `6` / `21` |
| `EXAMPLES_SKIP_RECENT_MINUTES` | no | `60` |
| `STREAM_REPLIES` | no | `true` (Telegram edit-in-place / stdio token stream) |
| `SHOW_THINKING` | no | `true` (needs `STREAM_REPLIES`) |
| `TOOL_TRACE` | no | `compact` (`compact`\|`full`\|`off`; needs `STREAM_REPLIES`) |
| `COALESCE_SETTLE_MS` | no | `2000` (`0` = off) |
| `SPINUP_NOTICE_MS` | no | `4000` (`0` = off) |
| `LOG_LEVEL` | no | `info` |

Source of truth is `internal/config/config.go`. Add new vars here in the same
change.

### MCP manifest (the one file)

Lists of processes don't fit env vars. TOML, mounted read-only. If a server is
listed, the agent gets it — the process composition **is** the grant.

```toml
[[server]]
name    = "google"
command = "google-mcp"
args    = ["--preset", "everyday"]
auth_args = ["auth"]
download_tag = "latest"
download_url = "https://github.com/shotah/google-mcp/releases/download/{tag}/google-mcp_{version}_{os}_{arch}.tar.gz"
# tools   = ["calendar_list_events"]  # optional allowlist
# exclude = ["raw_*"]                 # optional denylist
```

`download_url` + `download_tag` feed `gantry tools-fetch`. Placeholders:
`{os}` `{arch}` `{tag}` `{version}` (`version` = tag without leading `v`).
`gantry tools-plan` prints the resolved inventory without downloading.
Optional `auth_command` / `auth_args` drive `gantry auth <name>` and chat
`/auth` — [auth.md](auth.md), [deploy-docker.md](deploy-docker.md#mcp-tool-auth-browser-oauth).

Listed servers still **start**; `tools` / `exclude` only filter what is
**published** to the model. Tool names are always `{server}__{tool}`. Local
models often rewrite the prefix; the host repairs unambiguous mistakes.
Full contract: **[mcp.md](mcp.md)**.

### Host layout

Same three directories whether Docker bind-mounts them or systemd points at
`/opt/gantry/…`:

| Role | Typical path |
| --- | --- |
| Persona markdown | `PERSONA_DIR` → `/persona` or `/opt/gantry/persona` |
| MCP manifest | `MCP_MANIFEST` → `/etc/gantry/mcp.toml` or `/opt/gantry/mcp.toml` |
| SQLite + secrets | `DATA_DIR` → `/data` or `/opt/gantry/data` |

Only `PERSONA.md` (you) then `SELF.md` (the agent) load. Extra `*.md` is
ignored. Missing files are tolerated; empty persona is allowed but unusual.
Boot migrates leftover `SOUL.md` / `RULES.md` / `USER.md` / `TOOLS.md` into
`PERSONA.md` if needed, then deletes them. MCP tool names belong in the live
catalog, not persona — how to write one: [persona.md](persona.md).

## Agent loop & context bounds

This is the heart of the harness. Keep it boring and bounded. A long-horizon
agent is still one turn at a time; the loop is how those turns chain without
losing the plot:

1. **Assemble prompt**: `PERSONA.md` + `SELF.md` + memory hydration + session
   history (bounded) + user message. MCP names are **not** in persona.
2. **Call model** with MCP tool schemas (loaded eagerly at boot; refreshed on
   server restart; this is the live catalog).
3. **Tool iteration**: execute calls via MCP host (repair unambiguous prefix
   mistakes, else suggest closest real names *and* constrain the next call to
   them), truncate each result to `TOOL_RESULT_MAX_CHARS`, loop until final
   text or `TOOL_MAX_ITERATIONS`. At ~70% of the budget the model is told how
   many rounds remain; at the cap one landing call runs with tools withheld so
   the turn ends in a real reply. Each call appends a trace line to a
   streaming reply so long chains show motion.
4. **Reply** on the channel; append turn to session.

Every turn logs its own cost: `model call`, `tool done`, and `turn perf`
(`model_ms` / `tool_ms` / `total_ms`). On local models that split is the
difference between a prefill problem and a slow MCP —
[deploy-native.md](deploy-native.md#latency-measure-before-tuning).

| Mechanism | Behavior |
| --- | --- |
| History caps | Drop oldest past `HISTORY_MAX_MESSAGES` / `HISTORY_MAX_TOKENS` (chars/4 **estimate**). Prompt-only filler strip on user messages older than the last 40; assistant turns stay verbatim. SQLite is not rewritten. |
| Rolling summary | Trimmed turns fold into `session.summary` (`Facts:` + `Voice:`) via the same LLM; Voice copies forward; reinjected later |
| Tool truncate | Each MCP/memory tool result capped at `TOOL_RESULT_MAX_CHARS` |
| Tool collapse | Tool payloads older than the last 2 become one-line markers; matching tool-call args are stubbed. Session history never stores tool payloads. |
| Iteration cap | `TOOL_MAX_ITERATIONS` tool rounds, then one landing call with tools withheld |

`/new` wipes the session. `Voice:` folds into `SELF.md` (when self-notes are
enabled). `Facts:` park as a memory episode — `PERSONA.md` is operator-owned and
is never written. Existing memory rows stay.

### Self-notes (`SELF.md`)

Persona files describe who the agent **should** be. `SELF.md` is who it
**became** with you — and unlike chat history, it outlives `/new`.

- Lives in `PERSONA_DIR`, loaded in the stable prompt prefix (`PERSONA.md` →
  `SELF.md`).
- Mid-chat: `self_note` appends one short line.
- On history trim: new `Voice:` bits append the same way.
- On `/new`: distill **merges** into `SELF.md` (keep quoted jokes and
  nicknames). A bland tool session without Voice does not rewrite the file.
- Cap ~4KB; at capacity the tool (and trim append) refuse until distill or
  you prune.
- Needs a **writable** persona directory (`SELF_NOTES_ENABLED`, default on).
  Docker `:ro` silently disables the feature.
- **North-star aims** (how you show up for months) may live here as a few
  sentences. Progress, dates, and open loops are SQLite (`insight` /
  `aim/<area>`), not this file — [persona.md](persona.md#where-the-horizon-lives).

**Operator duty:** audit or delete `SELF.md` if the agent drifts.
[troubleshooting.md](troubleshooting.md#selfmd--personality-drift). Write a
tight `PERSONA.md` (examples, no MCP catalog): [persona.md](persona.md).

## Memory design

Long-horizon planning needs facts that outlive a session. Direction taken
from Google's Always-On Memory Agent (2026): **no embeddings, no vector DB —
an LLM writes structured rows into SQLite and a background job consolidates
them.** At personal-agent scale, structured + FTS5 beats ANN search and stays
greppable/deletable. Hand inspection: [memory.md](memory.md).

### Store

One SQLite file `$DATA_DIR/gantry.db` (WAL mode), pure-Go driver:

```sql
CREATE TABLE memory (
  id          INTEGER PRIMARY KEY,
  kind        TEXT NOT NULL,       -- fact | preference | person | episode | insight
  subject     TEXT NOT NULL,
  content     TEXT NOT NULL,
  source      TEXT NOT NULL,       -- chat | consolidation | operator
  confidence  REAL DEFAULT 1.0,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  expires_at  TEXT,
  superseded_by INTEGER
);
CREATE VIRTUAL TABLE memory_fts USING fts5(subject, content, content=memory);
```

### Builtin tools (only non-MCP tools)

| Tool | Role |
| --- | --- |
| `memory_store` | Atomic `kind` / `subject` / `content`. Months-scale plans: `insight` / `aim/<area>` ([persona.md](persona.md#where-the-horizon-lives)) |
| `memory_recall` | FTS5 + recency |
| `memory_forget` | By id or query — memory must be correctable |

Auto-save is **off**. Auto-saved hallucinations are worse than no memory. The
model stores deliberately; the consolidator promotes.

### Consolidation

A timer job (default 30 min, `0` disables) runs a bounded pass with the
**chat** LLM (same `LLM_*` Completer):

1. Read unconsolidated `episode` rows (batch of 20).
2. Extract durable `fact`/`preference`/`person`/`insight` rows.
3. On bad/empty model JSON, bump attempts and retry; quarantine after 3
   failures. Explicit `[]` marks the batch done.

Builtin backend only (`MEMORY_BACKEND=mcp:…` skips consolidator).

### Read path (hydration)

At session start and on `memory_recall`, hydrate at most ~30 rows: active
facts/preferences + FTS5 hits for the current message, rendered as a compact
`[memory]` block.

**Persona precedence is law**: anything in `PERSONA.md` outranks memory;
contradictions get surfaced, not obeyed.

### Why not vectors

- One user, one process: hundreds–thousands of rows, not millions. FTS5 +
  recency + kind filters is enough and is debuggable.
- Embeddings add a second model, cache, and dimension migration.
- Cloud vector stores add network, cost, and privacy surface to the most
  sensitive data in the system.
- Escape hatch: schema can grow an `embedding BLOB` later behind the same
  `memory_recall` interface.

## Ops surface

**The chat is the console.** A dashboard is a second interface — its own auth,
its own port (banned here), its own deploy story. Ops live in slash commands
and the tool trace in the reply bubble. Host-level questions (RAM/VRAM, GPU
residency) stay one `ssh` away ([observability.md](observability.md)).

| Command / signal | Behavior |
| --- | --- |
| `gantry run` | Daemon (default) |
| `gantry status` | Exit 0 if heartbeat fresh (≤ ~60s); Docker healthcheck |
| `gantry version` | Build ldflags |
| SIGTERM / Interrupt | Stop channel → drain in-flight turn → close MCP → close DB |
| Logs | JSON `slog` on stderr (`journalctl` / `docker logs`) |
| Chat cmds | `/new` `/cancel` `/status` `/tools` `/examples` `/perf` `/memstats` `/toolstats` `/tokens` `/auth` `/help` |
| SIGHUP | Reloads persona (unix) |
| Multi-bubble | Steer + settle (`COALESCE_SETTLE_MS`, default 2s): Completer cancelled, MCP kept |
| Spin-up notice | `SPINUP_NOTICE_MS` (default 4s) posts a working line before the first token |
| Photos | Inbound → vision; outbound `SendPhoto` for image URLs in the reply |

Telegram refreshes the `/` menu on every bot start (`setMyCommands`). Headless
tool OAuth: [auth.md](auth.md).

Dev: `make build|test|lint|run|ci|check`; `make install-hooks` for pre-commit.

No port is opened by the harness, ever.

## Packaging

- Go ≥ 1.26, single module, `CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`.
- Targets: `linux/amd64`, `linux/arm64`.
- Image: multi-stage → `gcr.io/distroless/static-debian12:nonroot` (ca-certs +
  tzdata, uid 65532, **no shell**). Healthchecks must use exec form
  (`["CMD","gantry","status"]`), never `CMD-SHELL`. MCP children must be
  static binaries too.
- CI: `go vet`, `golangci-lint`, `go test ./internal/... ./cmd/...` with
  coverage; on `main`, the badge is pushed to `gh-pages`.
- Release: `make release` (or `BUMP=minor|major` / `TAG=vX.Y.Z`);
  `.github/workflows/release.yml` runs GoReleaser on `v*` tags.
- Images: `.github/workflows/docker.yml` pushes multi-arch Distroless to
  `shotah/ai-gantry` (Hub) and `ghcr.io/shotah/ai-gantry` — `:edge` on `main`,
  `:latest` + semver on `v*` tags. Hub overview syncs from
  [`dockerhub.md`](dockerhub.md) (PNG banner; root readme is the pitch, not
  the Hub page).

## Decisions

Locked choices are summarized here; full rationale lives in
**[choices.md](choices.md)**.

1. **Name: ai-gantry 🏗️** — frame that holds tools; binary `gantry`. The
   industry name for that frame is **AI harness**; the product goal is
   **long-horizon planning**.
2. **Token counting: estimates** (chars/4), labeled as estimates.
3. **Memory: builtin SQLite, replaceable** via `MEMORY_BACKEND=mcp:<name>`.
4. **Streaming replies: on by default** (`STREAM_REPLIES=true`).
5. **Channel auth: allowlist only** — empty allowlist fails boot.
6. **Runtime image: distroless/static-debian12:nonroot** — MCP children static too.
7. **Logs on stderr** — stdout stays clean for the stdio REPL.

## Related

- [architecture.md](architecture.md) — diagrams and sequences
- [security.md](security.md) — threats and tradeoffs
- [choices.md](choices.md) — decision log
- [mcp.md](mcp.md) — tool naming and local REPL
