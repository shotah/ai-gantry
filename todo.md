# ai-gantry — TODO

Open follow-ups only. Shipped build order: [docs/milestones.md](docs/milestones.md).

---

## Channel: Signal (alongside Telegram)

### Decision (locked)

**One channel per container, chosen explicitly via `CHANNEL=`.**
Default stays **`telegram`** (real Bot API, single-process, already shipped).
Signal is **opt-in only**: `CHANNEL=signal` + Signal env — never auto-selected.

In-tree behind that switch — **not MCP, not a plugin loader.**

| Option | Verdict |
| --- | --- |
| **Default = telegram** | **Yes.** Explicit bot identity, long-poll, no sidecar. Unset/`CHANNEL=telegram` keeps today’s behavior. |
| **Opt-in = signal** | **Yes.** Operator must set `CHANNEL=signal` and Signal-required env (fail-fast). |
| **MCP channel** | **No.** MCP is model-invoked tools. The channel *drives* the agent loop, owns allowlist auth, session IDs, streaming `ReplyWriter`, and cron `Push`. |
| **Plugin / dynamic load** | **No.** `newChannel` already constructs only the selected transport. |
| **Both in one container** | **No.** One channel loop. Want both messengers → two containers. |

Signal has **no official Bot API**. Path is a **signal-cli** sidecar (JSON-RPC/REST) over localhost — multi-container by design, unlike Telegram.

### Implementation checklist

- [ ] **Spike** signal-cli JSON-RPC (or `signal-cli-rest-api`) receive + send from a throwaway Go client; document link/register flow (QR as secondary device preferred over registering a new number in CI).
- [ ] **Config** — add `signal` to allowed `CHANNEL` values; **default remains `telegram`**. When `CHANNEL=signal`, require e.g. `SIGNAL_CLI_URL` (or socket), `SIGNAL_ACCOUNT` (+E.164 / UUID), `SIGNAL_ALLOWED_USERS` (UUID and/or phone; allowlist only). Telegram env ignored unless `CHANNEL=telegram`.
- [ ] **`internal/channel/signal`** — implement `Channel` + `Pusher`; map sessions `signal:<account>:<peer>`; ignore non-allowlisted senders (log + drop).
- [ ] **Wire-up** — `newChannel` case + config validation (mirror Telegram fail-fast; no Signal code path unless chosen).
- [ ] **Text path** — inbound DM → `agent.Handle` → reply; outbound cron `Push` to allowlisted peer.
- [ ] **Commands** — `/new`, `/status`, `/tools` parity with Telegram/stdio.
- [ ] **Attachments (phase 2)** — inbound image → `channel.Image` / vision; outbound image if signal-cli attachment send is reliable.
- [ ] **Streaming (phase 2)** — Signal has no Telegram-style edit-in-place; either buffer full reply or send progressive messages (decide + document; default off).
- [ ] **Compose example** — sidecar service + volume for signal-cli data; kernel stays no inbound ports; document that Signal path is multi-container.
- [ ] **Docs** — readme env table, design non-goals update (“Telegram + Signal + stdio”), security (linked device = full account power; allowlist still required), LOCAL_AGENT notes.
- [ ] **Tests** — unit tests with fake JSON-RPC/SSE; no live Signal in CI.
- [ ] **Ops runbook** — link device, rotate, what breaks when signal-cli lags Signal-server, backup of signal-cli datadir.

### Explicit non-goals (Signal v1)

- Changing the default away from Telegram
- Running Telegram + Signal in one process
- Pairing / open inbox (allowlist only)
- Groups as primary UX (DMs first; groups later if needed)
- Auto-detecting Signal from env when `CHANNEL` is unset

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
