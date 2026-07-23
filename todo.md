# ai-gantry — TODO

Open follow-ups only. Shipped build order: [docs/milestones.md](docs/milestones.md).

---

## Channels — what unlocks adoption next

Telegram is a **real limiter** for many people (friends/work don’t live there), but
it’s also one of the *best* bot APIs for our model. The selling point isn’t
“more chat apps” — it’s **official bot identity + outbound-only + allowlist**,
same contract as today. Prefer platforms that fit that; don’t break “no ports.”

### Fit matrix (bot-friendly first)

| Priority | Channel | Official bot? | Inbound ports? | Fit for gantry | Why |
| --- | --- | --- | --- | --- | --- |
| **Shipped** | **Telegram** | Yes (Bot API) | No (long-poll) | ★★★★★ | Default; simplest personal bot story |
| **Shipped** | **Discord** | Yes (Bot + Gateway WS) | No (outbound WSS) | ★★★★★ | DMs; Message Content intent; same security story |
| **Shipped** | **Slack** | Yes (Socket Mode) | No (outbound WS) | ★★★★ | DMs + `@mention`; bot + app-level tokens |
| **P2** | **Signal** | **No** (signal-cli) | No* (sidecar) | ★★★ | Privacy crowd wants it; *not* a Bot API; multi-container; maintenance tax |
| Later | Matrix | Yes (Client-Server) | No (outbound sync) | ★★★ | Self-host crowd; more protocol surface |
| Avoid v1 | WhatsApp / Teams / Messenger | “Bot” via Cloud/Graph | **Usually yes** (webhooks) | ★ | Breaks no-ports; keep as documented non-goals |
| Avoid | iMessage / SMS as primary | No clean bot | Mixed | ★ | Carrier/webhook hell (see legacy `local-agent/docs`) |

\*Signal path needs a **signal-cli sidecar**; kernel stays closed, but deploy is no longer one process.

### Decision (locked for all new channels)

- One active channel per container: `CHANNEL=telegram|discord|slack|signal|stdio` (signal not shipped yet)
- Default stays **`telegram`**
- In-tree `internal/channel/<name>` — **not MCP, not plugins**
- Allowlist only (Discord user snowflakes / Slack user IDs / Signal UUIDs)
- DMs first; guild/channel mentions are phase 2 where relevant

### Pitch (shipped)

*“Personal MCP agent on Discord, Telegram, or Slack — still zero inbound ports.”*
Signal remains the privacy unlock ([P2](#p2--signal-checklist-after-discord)).

### Docs callouts (when implementing)

- [x] Readme “Who this is for” + non-goals: list **shipped / planned / won’t** channels with the matrix above (one short table)
- [x] Hello path: keep Telegram as fastest; add “Discord variant” compose snippet once P0 ships

### P0 — Discord checklist

- [x] Spike Gateway + DM receive/send in Go; Message Content intent; allowlist by user ID
- [x] Config: `CHANNEL=discord`, `DISCORD_BOT_TOKEN`, `DISCORD_ALLOWED_USERS` (snowflakes)
- [x] `internal/channel/discord` — `Channel` + `Pusher`; sessions `discord:<channel>:<user>`
- [x] Text cmds: `/new` `/status` `/tools` parity (agent-parsed; DMs)
- [x] Attachments phase 2 (vision in / images out)
- [x] Streaming phase 2 (edit message or buffer — Discord edits exist)
- [x] Tests with fake gateway; docs + example `.env` ([docs/discord.md](docs/discord.md))

### P1 — Slack checklist (Socket Mode only)

- [x] Spike Socket Mode (no Request URL); bot token + app-level token
- [x] Config: `CHANNEL=slack`, `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`, `SLACK_ALLOWED_USERS`
- [x] DMs / `app_mention`; thread → session id; cron `Push`
- [x] Docs: Socket Mode required (HTTP Events API = non-goal) — [docs/slack.md](docs/slack.md)
- [x] Files / streaming phase 2

### P2 — Signal checklist (after Discord)

- [ ] Spike signal-cli JSON-RPC receive/send; prefer link-as-secondary-device
- [ ] Config: `CHANNEL=signal`, `SIGNAL_CLI_URL`, `SIGNAL_ACCOUNT`, `SIGNAL_ALLOWED_USERS`
- [ ] `internal/channel/signal` + sidecar compose example
- [ ] Commands parity; attachments/streaming phase 2; ops runbook (cli expiry culture)
- [ ] Explicit: not a Bot API — document trust model (linked device ≈ full account)

### Explicit non-goals (channels)

- Opening inbound ports for WhatsApp Cloud / Teams webhooks
- Multi-channel in one process
- Pairing / open inbox
- Replacing Telegram as default

---

## Publish distroless image → Docker Hub

| Choice | Pick |
| --- | --- |
| Image | **`shotah/ai-gantry`** (+ `ghcr.io/shotah/ai-gantry`) |
| Workflows | [`docker.yml`](.github/workflows/docker.yml) + [`dockerhub-description.yml`](.github/workflows/dockerhub-description.yml) |

### Checklist

- [x] **Workflows** + readme pull docs
- [x] **Secrets** — `DOCKER_HUB_USERNAME`, `DOCKER_HUB_ACCESS_TOKEN` (Hub PAT needs **Read + Write + Delete** for README sync)
- [x] **First image push** — `edge` + multi-arch
- [x] **Hub README sync** — Delete scope on token fixed Forbidden
- [x] **Verify** — [hub.docker.com/r/shotah/ai-gantry](https://hub.docker.com/r/shotah/ai-gantry)

### Out of scope

- Baking MCP tools into the kernel image (see `local-agent/`)
- Replacing GoReleaser binary releases

---

## Fold local-agent appliance into this repo

Stop needing a second repo (`docker_open_claw` / `zeroclaw_scripts`) to run **our** stack.
Kernel = published distroless image; LOCAL_AGENT = in-tree appliance that bakes MCP tools.

### Decision

| Choice | Pick |
| --- | --- |
| Layout | **`local-agent/`** at repo root |
| Kernel image | **`shotah/ai-gantry`** (no MCP binaries) |
| local-agent image | **`gantry-local-agent:local`** (optional Hub later: `shotah/ai-gantry-local-agent`) |
| Examples | Slim `examples/personal-assistant/` stays kernel-only |

### Checklist

- [x] **Scaffold `local-agent/`** — Dockerfile, docker-compose, Makefile, mcp.toml, `.env.example`, scripts, docs, persona `*.example.md`, secrets stubs
- [x] **Wire docs** — root / examples / docs Path C → `local-agent/`
- [ ] **Smoke local** — `cd local-agent && make init && make build && make up` (needs your `.env` + Docker)
- [ ] **Dockerfile polish** — optionally `FROM shotah/ai-gantry:…` instead of curling the GitHub release tarball
- [ ] **Cutover live server** — point deploy path at in-repo `local-agent/`; smoke Telegram + one MCP tool
- [ ] **Optional CI / Hub** — build/publish `shotah/ai-gantry-local-agent` on tag
- [ ] **Archive secondary repo** — README “moved to ai-gantry/local-agent”; rename away from zeroclaw

### Non-goals

- Putting private OAuth tokens or real `SOUL.md` in git
- Making the default kernel image include Workspace/Strava/Garmin/…
- Rewriting auth scripts in Go on day one

---

## Nice-to-have (later)

- [x] Multimodal Telegram (inbound photo → vision request; outbound `SendPhoto`)
- [ ] Optional `embedding BLOB` behind the same `memory_recall` interface if FTS
      ever proves too weak at this scale
