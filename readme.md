# ai-gantry 🏗️

<p align="center">
  <img src="assets/banner.svg" alt="ai-gantry — the frame that holds the tools" width="100%">
</p>

<!-- Hub uses docs/dockerhub.md + assets/banner.png (SVG/mermaid break on Docker Hub). -->

<p align="center">
  <a href="https://github.com/shotah/ai-gantry/actions/workflows/ci.yml"><img src="https://github.com/shotah/ai-gantry/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/shotah/ai-gantry/actions/workflows/docker.yml"><img src="https://github.com/shotah/ai-gantry/actions/workflows/docker.yml/badge.svg" alt="Docker"></a>
  <a href="https://github.com/shotah/ai-gantry/actions/workflows/ci.yml"><img src="https://github.com/shotah/ai-gantry/raw/gh-pages/badges/coverage.svg" alt="Coverage"></a>
  <a href="https://hub.docker.com/r/shotah/ai-gantry"><img src="https://img.shields.io/docker/v/shotah/ai-gantry?logo=docker&label=docker%20hub" alt="Docker Hub"></a>
  <a href="https://hub.docker.com/r/shotah/ai-gantry"><img src="https://img.shields.io/docker/pulls/shotah/ai-gantry?logo=docker" alt="Docker pulls"></a>
</p>

> **gantry** *(n.)* — the rigid frame in a CNC machine or crane that holds and
> positions tools. The frame does nothing by itself; the tools do everything.

**A personal agent you can actually own.** One Distroless Go binary. One
persona. One OpenAI-compat model. MCP tools you choose. Chat that only dials
*out* (Telegram, Discord, or Slack). No dashboard. No config UI.
**No open ports. Ever.**

```text
static binary + persona + mcp.toml + any OpenAI-compat LLM  →  outbound chat
```

Chat, memory, and cron work with **zero MCP servers**. Tools are optional
binaries on `PATH` (or baked into an image) — the frame stays out of the way.

The whole kernel is ~12k lines of Go — small enough to read in an afternoon,
small enough to actually trust. It was hardened in production against
**small local models** (Qwen on Ollama on a mini-PC) as much as cloud
Flash: the loop assumes a small model *will* misspell a tool name, park its
answer in chain-of-thought, or print a tool call as plain text — and repairs
each of those instead of erroring the turn.

### Pull it (fastest hello)

Images publish on every `main` push and every `v*` tag to
[Docker Hub](https://hub.docker.com/r/shotah/ai-gantry) and GHCR:

| Tag | Meaning |
| --- | --- |
| `shotah/ai-gantry:latest` | Latest release |
| `shotah/ai-gantry:edge` | `main` (moving) |
| `shotah/ai-gantry:0.x.y` | Pinned release |
| `ghcr.io/shotah/ai-gantry:…` | Same tags on GHCR |

```bash
docker pull shotah/ai-gantry:latest
docker run --rm shotah/ai-gantry:latest version
```

Wire Telegram + Gemini via a **consumer template** (Hub image — no kernel
checkout): **[docs/deploy-docker.md](docs/deploy-docker.md)** →
[`examples/docker/`](examples/docker/). Cloud VMs: [`examples/hosting/`](examples/hosting/)
([GCP](examples/hosting/gcp/) · [AWS](examples/hosting/aws/)).

### Kernel vs appliance

| | **Kernel** (`gantry`) | **Appliance** ([`local-agent/`](local-agent/)) |
| --- | --- | --- |
| What | Runtime only — env + mounts | Kernel + Workspace / Strava / Garmin / Cast / YT Music / search |
| Run it | **Hub image**, binary, or systemd | Native Linux + Ollama, or Docker compose |
| Start here if | You want a tiny host you control | You want a full life-stack assistant |

> **In production** as a native appliance (Telegram + local Qwen via Ollama +
> MCP). Same kernel ships on Hub for cloud LLMs (Gemini/Grok). Not a demo
> scaffold — a binary with real deploy stories.

### Who this is for

Self-hosters and local-LLM operators who want an **outbound-only** assistant,
**inspectable** SQLite memory, and **MCP as the only plugin surface** — not
another multi-agent platform.

| Pick **gantry** when… | Pick something else when… |
| --- | --- |
| You want small, boring, shippable | You need a web UI or team workspace |
| Allowlist + no inbound ports is the security story | You need WhatsApp / Teams inbound webhooks |
| Local models must finish tool turns | You want multi-agent routing / pairing flows |
| Env + mounts is enough config | You want a dashboard or no-code canvas |

Longer ICP / competition / evangelism notes:
**[docs/positioning.md](docs/positioning.md)**.

| Status | Channel | Notes |
| --- | --- | --- |
| **Shipped** | Telegram (default) | Fastest hello path; long-poll |
| **Shipped** | Discord | DMs; Gateway WS — [docs/discord.md](docs/discord.md) |
| **Shipped** | Slack | Socket Mode only — [docs/slack.md](docs/slack.md) |
| **Planned** | Signal | Sidecar (`signal-cli`); not a Bot API |
| **Won’t** | WhatsApp / Teams / Messenger webhooks | Need inbound ports — breaks the model |

One `CHANNEL` per process. Allowlist only; no pairing.

### Chat is the console (no dashboard)

Ops and tool login happen **in the same chat** you already use — allowlisted,
on your phone, zero inbound ports. Telegram refreshes the `/` menu on every
bot start (`setMyCommands`); type `/help` anytime.

| Command | What it does |
| --- | --- |
| `/status` `/perf` `/memstats` `/toolstats` | Session bounds, last-turn timing, memory health, MCP ledger |
| `/tools` `/examples` `/new` `/cancel` `/help` | Catalog, capability ideas (`/examples on` / `off`), reset session (**distills personality into `SELF.md`**), abort in-flight turn |
| **`/auth`** | **Headless MCP OAuth** — paste a code from a static catch page; no laptop `localhost` callback |

Headless Google / Strava / Health: `/auth google` (etc.) → approve → paste
code. Laptop still works via `make *-auth`. Guide:
**[docs/auth.md](docs/auth.md)**. Observability from chat + host:
**[docs/observability.md](docs/observability.md)**.

### Personality that survives `/new`

Most agents *feel* like someone after a long chat — then you hit `/new` (or
history rolls off) and the funny, game-playing, in-joke version is gone.
Gantry keeps that growth on purpose:

| Piece | What it does |
| --- | --- |
| **`SELF.md`** | Agent-writable notes in `PERSONA_DIR` — voice, humor, running jokes, rituals |
| **`self_note`** | Builtin tool: jot one short line when personality happens mid-chat |
| **Distill on `/new`** | Before the session wipe, one model pass rewrites `SELF.md` from the dying chat + existing notes |

Operator files (`SOUL.md` / `RULES.md` / `USER.md` / `TOOLS.md`) stay yours.
`SELF.md` is the only file the agent may write — capped (~4KB), greppable,
diffable, and yours to prune. Docker mounts `./persona` **writable** for this;
`:ro` silently disables the feature (see
**[docs/troubleshooting.md](docs/troubleshooting.md#selfmd--personality-drift)**).

**You own the veto.** If the agent gets snarky, clingy, or just “not them”
anymore, open `SELF.md` and delete lines — or wipe the file and start fresh.
Treat it like you would a friend’s inside jokes: keep what’s good, cut what
isn’t. Audit after long sessions or whenever the vibe feels off.

### Hardened for small local models

Most agent stacks are tuned against frontier cloud models and quietly assume
tool calling just works. Gantry is hardened where 4–30B local models actually
fail — and refuses the platform tax (huge tool catalogs, embedding
round-trips, gateways, dashboards) that makes small models worse at picking
tools in the first place.

The same levers pay for themselves on frontier models: tool schemas, history,
and tool results are re-billed on **every** turn, so a filtered tool surface,
bounded history, and truncated results cut prompt tokens and latency — and
name repair turns would-be failed rounds into completed calls instead of
another billed retry.

| Lever | What we do | Why it matters |
| --- | --- | --- |
| Tool surface | Manifest filters + MCP `--tool-tier` | Smaller schemas → better tool picks (Flash *or* Qwen) |
| Name repair | Prefix alias/rebuild, closest-name hints, then a grammar-constrained retry | `google_search__…` and `mcp__get_hrv` still land; an unresolvable name makes the retry unable to misspell it |
| Think stalls | Promote CoT → reply after tools | Multi-step turns finish instead of ERROR |
| Printed calls | Parse a tool call written as text and run it | A model that prints `{"name":…}` never speaks JSON at you |
| Multi-bubble | Interrupt + coalesce + settle (`COALESCE_SETTLE_MS`) | “Strava… wait Garmin… nvm calendar” → one joined turn |
| Slow turns | Per-turn perf logs + `/perf` / tool trace in chat | Know whether prefill, thinking, or an MCP is the wait |
| Memory | SQLite + FTS5 in-process | No embedding API before every reply |
| Personality | `SELF.md` + `self_note` + distill on `/new` | The funny agent survives resets — and you can prune it |
| Runtime | One static binary (systemd *or* Distroless) | No Node/Bun/gateway in the path |
| Gemini 3 | Preserves `thought_signature` on tool rounds | Cloud multi-step turns don't 400 |

The same discipline runs through the MCP fleet: every tool server is a static
Go binary with one contract — stdio transport, an auth subcommand (CLI *or*
chat `/auth`), GoReleaser releases that `gantry tools-fetch` can pin — and
every tool reaches the model under one uniform `{server}__{tool}` name. One
convention for the model to learn; one repair path when a small model bends it.

Details: [docs/mcp.md](docs/mcp.md) · [docs/deploy-native.md](docs/deploy-native.md).

---

## Start here

| Path | When | Doc |
| --- | --- | --- |
| **Docker Hub + cloud LLM** *(fastest)* | Consumer repo + Hub image | **[examples/docker/](examples/docker/)** · [deploy-docker](docs/deploy-docker.md) |
| **Native + local model** | Consumer repo + release binary + systemd | **[examples/native/](examples/native/)** · [deploy-native](docs/deploy-native.md) |
| **GCP GCE** | Consumer repo on a small always-on VM | **[examples/hosting/gcp/](examples/hosting/gcp/)** |
| **AWS EC2** | Consumer repo on a small always-on instance | **[examples/hosting/aws/](examples/hosting/aws/)** |
| **REPL** | Hack on the binary | `make init && make run` (`CHANNEL=stdio`) |

Full life-stack (tools + auth helpers): **[local-agent/](local-agent/)**.  
**MCP login:** chat `/auth` on a headless box (**[docs/auth.md](docs/auth.md)**),
or laptop browser callback
(**[deploy-docker § MCP tool auth](docs/deploy-docker.md#mcp-tool-auth-browser-oauth)**).  
Cookbook: **[examples/README.md](examples/README.md)**. Positioning /
design / security / MCP / troubleshooting (`SELF.md`): **[docs/](docs/)**.

---

## Reference

Deep contract below — principles, env table, memory, packaging. Skim if you
already have a bot running; read before you grant MCP tools or expose an
allowlist to friends.

## 1. Problem statement

Platform agent stacks drift toward multi-agent products: multiple providers,
dashboards, console features, config UI. Our deployment model is the opposite
(pitch + ICP: [docs/positioning.md](docs/positioning.md)):

```text
process = persona + model + MCP set + data dir
```

Want another LLM or persona? Another process (second systemd unit or compose
service). No in-process routing, no dashboard, no manual config surface — a
kernel that does exactly that and nothing else.

## 2. Design principles

1. **Stupid simple.** One agent, one model, one channel loop. If a feature
   needs a diagram to explain, it probably belongs in an MCP binary, not here.
2. **Highly performant.** Pure Go, static binary, no CGO, small RSS, no
   background frameworks. Long-poll + goroutines; nothing dials in.
3. **Highly portable.** `CGO_ENABLED=0` static binary — runs under systemd or
   Distroless (no shell in the image). No glibc dependency in our binary.
4. **Plugin-centric.** Capabilities come from external binaries over MCP
   stdio. The gantry hosts tools; it does not implement them. Import libraries
   over writing our own (official MCP SDK, maintained Telegram lib, pure-Go
   SQLite).
5. **1:1, always.** No multi-provider config, no multi-agent config, no peer
   routing. Scaling = more processes (compose services or systemd units).
6. **Env + files is the config plane.** Secrets and scalars via env. Structure
   via persona markdown, MCP manifest, and a data directory (bind-mounts in
   Docker; paths on the host for native).
7. **Memory is structured and inspectable.** SQLite rows you can read and
   delete with `sqlite3`, not opaque embedding blobs. Persona files always
   outrank recalled memory.

## 3. Architecture

```mermaid
flowchart LR
  TG[Telegram] <-->|long poll, outbound only| K

  subgraph Host["host or Distroless container"]
    K[gantry binary]
    M1[mcp binary A]
    M2[mcp binary B]
    K -->|MCP stdio| M1
    K -->|MCP stdio| M2
  end

  K -->|OpenAI-compat| LLM[one LLM endpoint]
  K --- P[("persona/*.md")]
  K --- D[("data/gantry.db")]
  M1 --- S[("secrets / .config")]
```

Deploy shapes: [native](docs/deploy-native.md) · [Docker](docs/deploy-docker.md).

### 3.1 Process model

One OS process. Goroutines:

| Goroutine | Job |
| --- | --- |
| channel poller | Telegram `getUpdates` long-poll, allowlist filter |
| agent loop | per-message: assemble prompt → model → tool calls → reply |
| MCP supervisors | one per server: spawn, health, restart w/ backoff |
| memory consolidator | optional timer job (see §6) |

No goroutine talks to the network inbound. Healthcheck is `gantry status`
(exit-code) reading a heartbeat row in SQLite — no port needed.

### 3.2 Package layout (single module)

```text
cmd/gantry/          main: run | init | auth | status | version
internal/config/     env parsing + validation, fail-fast at boot
internal/channel/    Channel interface; telegram/, stdio/ (test/dev)
internal/provider/   ONE implementation: OpenAI-compatible chat client
internal/mcp/        stdio host: spawn, list tools, call, truncate, restart
internal/agent/      the loop: prompt assembly, tool iteration, caps
internal/session/    bounded history, /new reset, rolling summary
internal/memory/     SQLite structured memory + FTS5 + consolidation
internal/persona/    load + concat markdown from /persona
internal/heartbeat/  SQLite heartbeat for `gantry status`
internal/drain/      wait for in-flight turn on shutdown
internal/cron/       scheduled turns → agent → channel push
```
(Diagrams + sequences: [docs/architecture.md](docs/architecture.md).)

### 3.3 Dependencies (import over write)

| Concern | Library | Why |
| --- | --- | --- |
| MCP client | `github.com/modelcontextprotocol/go-sdk` | Official SDK; stdio transport, schema handling |
| SQLite | `modernc.org/sqlite` | Pure Go (no CGO), FTS5 works, one file DB |
| Telegram | `github.com/go-telegram/bot` | Zero-dep, maintained, long-poll native |
| LLM client | `github.com/openai/openai-go/v3` | Official; custom `base_url` covers Gemini's OpenAI-compat endpoint, xAI, Ollama, etc. |
| Env config | `github.com/caarlos0/env/v11` | Struct tags → env, tiny |
| MCP manifest | `github.com/pelletier/go-toml/v2` | Minimal TOML for `mcp.toml` |
| Logging | stdlib `log/slog` | JSON to **stderr** (keeps stdio REPL clean; journald / `docker logs`) |

One provider implementation (OpenAI-compatible) is deliberate: Gemini, Grok,
and local models all speak it. Model identity is just `LLM_BASE_URL` +
`LLM_MODEL` + `LLM_API_KEY`. No provider registry.

## 4. Configuration contract

Everything is env or a mount. No config UI, no `config set`, no sync step.

### 4.1 Environment variables

| Var | Required | Example / default |
| --- | --- | --- |
| `LLM_BASE_URL` | yes | `https://generativelanguage.googleapis.com/v1beta/openai` |
| `LLM_API_KEY` | yes | — |
| `LLM_MODEL` | yes | `gemini-3.5-flash` |
| `LLM_MAX_TOKENS` | no | `4096` (completion output cap; `0` = provider default) |
| `TELEGRAM_BOT_TOKEN` | yes (telegram) | — |
| `TELEGRAM_ALLOWED_USERS` | yes (telegram) | `123456789,987654321` (numeric IDs; **allowlist only — no pairing**) |
| `TELEGRAM_ERROR_REPORTING` | no | `off` (`off`\|`error`\|`warn` — tee slog into the SAM chat as expandable HTML) |
| `DISCORD_BOT_TOKEN` | yes (discord) | — |
| `DISCORD_ALLOWED_USERS` | yes (discord) | snowflake user IDs; **allowlist only** — see [docs/discord.md](docs/discord.md) |
| `SLACK_BOT_TOKEN` | yes (slack) | `xoxb-…` bot token |
| `SLACK_APP_TOKEN` | yes (slack) | `xapp-…` app-level token (`connections:write`) — [docs/slack.md](docs/slack.md) |
| `SLACK_ALLOWED_USERS` | yes (slack) | Slack member IDs; **allowlist only** |
| `CHANNEL` | no | `telegram` (default), `discord`, `slack`, or `stdio` |
| `PERSONA_DIR` | no | `/persona` |
| `DATA_DIR` | no | `/data` |
| `MCP_MANIFEST` | no | `/etc/gantry/mcp.toml` |
| `HISTORY_MAX_MESSAGES` | no | `200` |
| `HISTORY_MAX_TOKENS` | no | `128000` |
| `TOOL_RESULT_MAX_CHARS` | no | `6000` |
| `TOOL_MAX_ITERATIONS` | no | `10` (tool rounds per turn; at the cap a final no-tools call forces a text reply) |
| `TOOL_SCHEMA_MAX_TOKENS` | no | `0` (log estimate only; `>0` = hard fail if over) |
| `SELF_NOTES_ENABLED` | no | `true` (agent-writable `SELF.md` via `self_note` tool + personality distill on `/new`; auto-off when `PERSONA_DIR` is read-only) |
| `MEMORY_ENABLED` | no | `true` |
| `MEMORY_BACKEND` | no | `builtin` (or `mcp:<server-name>`, see §6 / §9) |
| `MEMORY_CONSOLIDATE_MINUTES` | no | `30` (`0` = off; builtin backend only) |
| `CRON_ENABLED` | no | `true` |
| `CRON_TZ` | no | `America/Los_Angeles` (Pacific; override with any IANA zone) |
| `CRON_MAX_JOBS` | no | `50` |
| `CRON_TICK_SECONDS` | no | `15` |
| `STREAM_REPLIES` | no | `true` (Telegram edit-in-place / stdio token stream) |
| `SHOW_THINKING` | no | `true` (Telegram CoT italics → expandable blockquote; needs `STREAM_REPLIES`; independent of `LLM_REASONING_EFFORT`) |
| `TOOL_TRACE` | no | `compact` (`compact` = `Making Calls: ✓, ✗`; `full` = → name / ✓ timing; `off` = hide; needs `STREAM_REPLIES`) |
| `COALESCE_SETTLE_MS` | no | `2000` (quiet ms after a bubble **interrupts a running turn**, before one joined turn; a lone message never waits; `0` = off) |
| `SPINUP_NOTICE_MS` | no | `4000` (post “working on it” after this much model silence; the first turn after start posts at once; needs `STREAM_REPLIES`; `0` = off) |
| `LOG_LEVEL` | no | `info` |

Boot is fail-fast: missing required env = clear error + exit 1. No partial
starts, no interactive setup.

### 4.2 MCP manifest (the one file)

Lists of processes don't fit env vars; this is the single structured file,
mounted read-only. TOML, minimal:

```toml
[[server]]
name    = "google"
command = "google-mcp"
args    = ["--preset", "everyday"]
auth_args = ["auth"]
download_tag = "latest"   # or pin "v1.0.0"; "latest" resolves via GitHub API at plan time
download_url = "https://github.com/shotah/google-mcp/releases/download/{tag}/google-mcp_{version}_{os}_{arch}.tar.gz"

[[server]]
name    = "garmin"
command = "garmin"
args    = ["mcp"]
auth_args = ["login"]     # optional; `gantry auth garmin`
tools   = ["get_sleep", "get_weight", "get_hrv"]  # optional allowlist
# exclude = ["raw_*"]                               # optional denylist
# tools_prefix = "garm"                             # optional; default name
download_tag = "latest"
download_url = "https://github.com/shotah/go-garmin/releases/download/{tag}/garmin_{version}_{os}_{arch}.tar.gz"

[[server]]
name    = "strava"
command = "strava-mcp"
auth_args = ["auth"]
download_tag = "latest"
download_url = "https://github.com/shotah/go-strava-mcp/releases/download/{tag}/strava-mcp_{version}_{os}_{arch}.tar.gz"
```

`download_url` + `download_tag` feed `gantry tools-fetch` (native deploy and
Docker bake): placeholders `{os}` `{arch}` `{tag}` `{version}` (`version` =
tag without leading `v`). `gantry tools-plan` prints the resolved inventory
without downloading. Optional `auth_command` / `auth_args` drive
`gantry auth <name>` — for browser OAuth, run that on the machine with your
browser ([deploy-docker § MCP tool auth](docs/deploy-docker.md#mcp-tool-auth-browser-oauth)).

Listed servers still **start**; `tools` / `exclude` only filter what is
**published** to the model (boot logs `tools_listed` vs `tools_published`).
Schema cost is logged as `est_tokens` (chars/4); set `TOOL_SCHEMA_MAX_TOKENS`
to hard-fail when the published set is too fat.

No bundles/grants layer: if a server is in the manifest, the agent gets it.
The process composition IS the grant (1:1 — you chose this persona + MCP set
on purpose).

Tool names are always prefixed `{server}__{tool}` (OpenAI-safe; avoids
collisions). Local models often turn the hyphenated *prefix* into underscores
(`google_search__google_search`), or invent one outright (`mcp__get_hrv`); the
host repairs both back to the catalog name when exactly one tool can be meant,
and on hard misses returns a model-facing suggestion naming the closest real
tools. Full contract, `/tools` REPL workflow, and why:
**[docs/mcp.md](docs/mcp.md)**.

### 4.3 Host layout

Same three directories whether Docker bind-mounts them or systemd points at
`/opt/gantry/…`:

| Role | Typical path |
| --- | --- |
| Persona markdown | `PERSONA_DIR` → `/persona` or `/opt/gantry/persona` |
| MCP manifest | `MCP_MANIFEST` → `/etc/gantry/mcp.toml` or `/opt/gantry/mcp.toml` |
| SQLite + secrets | `DATA_DIR` → `/data` or `/opt/gantry/data` |

Compose sample + Hub hello: [docs/deploy-docker.md](docs/deploy-docker.md).  
systemd + Ollama: [docs/deploy-native.md](docs/deploy-native.md).

## 5. The agent loop (context management)

This is the part that earns its keep. Keep it boring and bounded:

1. **Assemble prompt**: persona markdown (concat, fixed order) + memory
   hydration block (§6.4) + session history (bounded) + user message.
2. **Call model** with MCP tool schemas (loaded eagerly at boot; refreshed on
   server restart).
3. **Tool iteration**: execute calls via MCP host (repair unambiguous prefix
   mistakes, else suggest closest real names *and* constrain the next call to
   them with a response-format grammar), truncate each result to
   `TOOL_RESULT_MAX_CHARS`, loop until final text or `TOOL_MAX_ITERATIONS`.
   At ~70% of the budget the model is told how many rounds remain; at the cap
   one landing call runs with tools withheld so the turn ends in a real reply
   (what was done, what's left) instead of an error that drops the work.
   Each call appends a trace line (`→ name`, `✓ 1.2s · 4.1k chars`) to a
   streaming reply so long chains show motion.
4. **Reply** on the channel; append turn to session.

Every turn logs its own cost: `model call` (`first_token_ms`, `dur_ms`,
`prompt_est_tokens`, `tool_schemas`), `tool done` (`dur_ms`, `result_chars`),
and `turn perf` (`model_ms` / `tool_ms` / `total_ms`). On local models that
split is the difference between a prefill problem and a slow MCP —
[docs/deploy-native.md](docs/deploy-native.md#latency-measure-before-tuning).

Bounding rules:

- Hard cap `HISTORY_MAX_MESSAGES`; drop oldest turns past `HISTORY_MAX_TOKENS`.
  Token counts are chars/4 **estimates** and are labeled as such everywhere
  they surface (logs, `/status`) — see §9. Persona + last N turns are
  always protected.
- When history is trimmed, dropped turns fold into a persistent per-session
  `summary` paragraph via the same LLM (one string — not a framework). The
  summary is injected as a system block on later turns.
- Tool results older than the last 4 collapse to one line:
  `[tool gmail.search: N chars, truncated]`.
- `/new` wipes the session (memory untouched). When self-notes are enabled,
  a distill pass rewrites `SELF.md` first so personality survives the reset.

### 5.1 Self-notes (`SELF.md`) — grown personality

Persona files describe who the agent **should** be. `SELF.md` is who it
**became** with you — and unlike chat history, it outlives `/new`.

- Lives in `PERSONA_DIR`, loaded in the stable prompt prefix (`SOUL` → `SELF`
  → `RULES` → `USER` → `TOOLS`).
- Mid-chat: `self_note` appends one short line (model already sees the full
  file in the persona, so it can skip duplicates).
- On `/new`: full rewrite distill (keep what matters, fold in the dying
  session, ≤30 bullets) — not a blind append.
- Cap ~4KB; at capacity the tool refuses until distill or you prune.
- Needs a **writable** persona directory (`SELF_NOTES_ENABLED`, default on).

**Operator duty:** audit or delete `SELF.md` if the agent drifts into
behavior you don’t want. Details + recipes:
**[docs/troubleshooting.md](docs/troubleshooting.md#selfmd--personality-drift)**.

## 6. Memory design

Direction taken from Google's Always-On Memory Agent (2026): **no embeddings,
no vector DB — an LLM writes structured rows into SQLite and a background job
consolidates them.** At personal-agent scale, structured + FTS5 beats ANN
search and stays greppable/deletable. (Meta/OpenAI memory products converge on
the same shape: typed facts + episodic notes + periodic distillation.)

### 6.1 Store

One SQLite file `$DATA_DIR/gantry.db` (WAL mode), pure-Go driver:

```sql
CREATE TABLE memory (
  id          INTEGER PRIMARY KEY,
  kind        TEXT NOT NULL,       -- fact | preference | person | episode | insight
  subject     TEXT NOT NULL,       -- "chris", "climbing", "mom"
  content     TEXT NOT NULL,       -- one atomic statement
  source      TEXT NOT NULL,       -- chat | consolidation | operator
  confidence  REAL DEFAULT 1.0,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  expires_at  TEXT,                -- TTL per kind (episodes decay, facts don't)
  superseded_by INTEGER            -- consolidation links, never silent delete
);
CREATE VIRTUAL TABLE memory_fts USING fts5(subject, content, content=memory);

CREATE TABLE session (...);        -- bounded history + rolling summary
CREATE TABLE heartbeat (...);      -- for `gantry status`
```

### 6.2 Write path

The model gets three built-in tools (the only non-MCP tools in the gantry):

- `memory_store(kind, subject, content)` — atomic statements only
- `memory_recall(query)` — FTS5 + recency-ranked
- `memory_forget(id | query)` — hard requirement; memory must be correctable

Auto-save is **off by default**. Auto-saved hallucinations (wrong emails) are
worse than no memory. The model stores deliberately; the consolidator promotes.

### 6.3 Consolidation (the Google idea)

A timer job (default 30 min, `0` disables) runs a bounded pass with the **chat**
LLM (same `LLM_*` Completer — no separate consolidator model):

1. Read unconsolidated `episode` rows (batch of 20).
2. Extract durable `fact`/`preference`/`person`/`insight` rows; optional
   `supersedes` links are limited to IDs in that batch.
3. On bad/empty model JSON, bump attempts and retry; quarantine after 3 failures
   (never mark success on parse fail). Explicit `[]` marks the batch done.

Builtin backend only (`MEMORY_BACKEND=mcp:…` skips consolidator and logs a warn).
Fully skippable via `MEMORY_CONSOLIDATE_MINUTES=0`. This is our "sleep cycle."

### 6.4 Read path (hydration)

At session start and on `memory_recall`, hydrate at most ~30 rows:
active facts/preferences (non-expired, non-superseded) + FTS5 hits for the
current message, rendered as a compact block:

```text
[memory]
- (person) mom: prefers calls over texts
- (preference) user: coaching tone, no fluff
```

**Persona precedence is law**: anything in `USER.md` outranks memory;
contradictions get surfaced, not obeyed.

### 6.5 Why not vectors / cloud vector storage

- One user, one process: recall corpus is hundreds–thousands of rows, not
  millions. FTS5 + recency + kind filters is enough and is debuggable.
- Embeddings add a second model dependency, cache, and dimension migration
  for marginal recall gain at this scale.
- Cloud vector stores add network, cost, and privacy surface to the most
  sensitive data in the system.
- Escape hatch: schema reserves the option of an `embedding BLOB` column
  later. If recall quality ever demonstrably hurts, add it then — behind the
  same `memory_recall` interface, no design change.

## 7. Ops surface

**The chat is the console.** A dashboard is a second interface — its own
auth, its own port (banned here), its own deploy story — and it isn't with
you when a turn feels slow. The chat already is: allowlisted, on your phone,
and the exact place the question comes up. So ops lives in slash commands
and the tool trace in the reply bubble, and host-level questions (RAM/VRAM,
GPU residency) stay one `ssh` away
([docs/observability.md](docs/observability.md)) instead of becoming a web
UI. ChatOps is not a compromise for an agent — it is the point.

- `gantry run` — the daemon (default)
- `gantry status` — exit-code healthcheck (reads `heartbeat` row in `$DATA_DIR/gantry.db`)
- `gantry version` — build info
- Logs: JSON `slog` to stderr (`journalctl` native, `docker logs` in compose).
- Consumption & timing without a dashboard — RAM/VRAM (`ollama ps`, not `top`),
  per-turn `jq` recipes, `docker stats`: **[docs/observability.md](docs/observability.md)**.
- Telegram/stdio slash commands (also in the pitch above): `/new` `/cancel`
  `/status` `/tools` `/examples` `/perf` `/memstats` `/toolstats` `/auth` `/help`;
  Telegram menu is pushed via `setMyCommands` on connect. Unix `SIGHUP`
  reloads persona. Headless tool OAuth: **[docs/auth.md](docs/auth.md)**.
- **Multi-bubble (interrupt → coalesce → settle):** a lone message runs at once;
  a follow-up sent while a turn is running cancels the current loop, joins the
  bubbles into one user message, waits `COALESCE_SETTLE_MS` (default **2000**)
  of quiet, then resubmits as a single turn. `/cancel` also clears a pending
  settle batch. Tools that already finished are not undone. Cron/reaction
  synthetics skip this path. Details:
  [local-agent/docs/telegram.md](local-agent/docs/telegram.md).
- **Spin-up notice:** local models prefill in silence, so `SPINUP_NOTICE_MS`
  (default **4000**) opens the streaming bubble with a status line before the
  first token — at once on the first turn after start (known-cold: model load
  and/or empty prompt cache), otherwise only if the turn stays silent that long
  (a prompt-cache miss, which no provider API exposes). The line is transient —
  the reply replaces it, unlike a tool trace. Needs `STREAM_REPLIES`.
- Telegram photos: inbound → vision (Gemini/OpenAI-compat); outbound `SendPhoto` when the
  reply includes a markdown image or `*.png`/`*.jpg`/… URL (caption = remaining text).
- Dev: `make build|test|lint|run|ci|check`; `make install-hooks` for pre-commit
  (autofix + lint + test; same shape as go-garmin).

That's the entire ops/UI story. No port is opened by the gantry, ever.

## 8. Build & packaging

- Go ≥ 1.26, single module, `CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`.
- Targets: `linux/amd64`, `linux/arm64`.
- Image: multi-stage — build gantry (and later copy MCP tool binaries in),
  final `FROM gcr.io/distroless/static-debian12:nonroot` (ca-certs + tzdata,
  uid 65532, **no shell**). Healthchecks must use exec form
  (`["CMD","gantry","status"]`), never `CMD-SHELL`.
  MCP children must be static binaries too — there is no libc/shell to lean on.
- CI: `go vet`, `golangci-lint`, `go test ./internal/... ./cmd/...` with coverage; on `main`,
  the badge is pushed to `gh-pages` as `badges/coverage.svg` (README uses the `raw/gh-pages` URL).
- Release: `make release` (or `BUMP=minor|major` / `TAG=vX.Y.Z`) bumps
  `VERSION`, tags, and pushes; `.github/workflows/release.yml` runs GoReleaser
  on `v*` tags (same flow as the other shotah MCP repos).
- Images: `.github/workflows/docker.yml` pushes multi-arch Distroless to
  `shotah/ai-gantry` (Hub) and `ghcr.io/shotah/ai-gantry` — `:edge` on `main`,
  `:latest` + semver on `v*` tags. Hub overview syncs from
  [`docs/dockerhub.md`](docs/dockerhub.md) (PNG banner; root readme is too
  large / SVG-heavy for Hub).

## 9. Decisions

Locked choices are summarized here; full rationale and rejected alternatives
live in **[docs/choices.md](docs/choices.md)**.

1. **Name: ai-gantry 🏗️** — frame that holds tools; binary `gantry`.
2. **Token counting: estimates** (chars/4), labeled as estimates.
3. **Memory: builtin SQLite, replaceable** via `MEMORY_BACKEND=mcp:<name>`.
4. **Streaming replies: on by default** (`STREAM_REPLIES=true`; edit-in-place where the channel supports it; set `false` for a single final bubble).
5. **Channel auth: allowlist only** — empty allowlist fails boot (Telegram / Discord / Slack).
6. **Runtime image: distroless/static-debian12:nonroot** — MCP children static too.
7. **Logs on stderr** — stdout stays clean for the stdio REPL.

Architecture diagrams / sequences: [docs/architecture.md](docs/architecture.md).
Security tradeoffs & residual risks: [docs/security.md](docs/security.md).
Design deep-dive: [docs/design.md](docs/design.md).

## License

MIT — see [LICENSE](LICENSE).
