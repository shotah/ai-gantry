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

### Pros / cons

**In-tree `CHANNEL=signal` (chosen)**
- Pros: matches Telegram; cron/stream/photos stay in one process; allowlist at the edge; compose stays obvious.
- Cons: Signal deps/docs/ops live in this repo; signal-cli must stay patched (~3 month Signal client expiry culture).

**MCP “send/receive Signal” tools**
- Pros: keeps kernel “pure.”
- Cons: model would own I/O (wrong); inbound loop + allowlist + cron push don’t fit; latency/token waste.

**Both channels active at once**
- Pros: one bot, two messengers.
- Cons: session ID collisions, which allowlist, dual stream state, harder shutdown; better as two containers sharing nothing (or carefully sharing `DATA_DIR` — usually don’t).

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
- [ ] **Docs** — readme env table, design non-goals update (“Telegram + Signal + stdio”), security (linked device = full account power; allowlist still required), Tim/`personal-assistant` notes.
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

Kernel image already builds locally (`Dockerfile` → `gcr.io/distroless/static-debian12:nonroot`).
GoReleaser ships **binaries** on `v*` tags; **nothing pushes the container** yet.

Reference workflow that already deploys to your Hub account:
[`hytale-server-container/.github/workflows/main.yml`](../hytale-server-container/.github/workflows/main.yml)
→ `shotah/hytale-server` via `DOCKER_HUB_USERNAME` + `DOCKER_HUB_ACCESS_TOKEN`.

Hub namespace today: [hub.docker.com/repositories/shotah](https://hub.docker.com/repositories/shotah)
(`hytale-server`, `nolfo`, … — **no gantry/ai-gantry repo yet**).

### Decision

| Choice | Pick |
| --- | --- |
| Image name | **`shotah/ai-gantry`** (+ `ghcr.io/shotah/ai-gantry`) |
| Workflows | [`.github/workflows/docker.yml`](.github/workflows/docker.yml) + [`dockerhub-description.yml`](.github/workflows/dockerhub-description.yml) |
| Triggers | `v*` → semver + `latest`; `main` → `edge` + `sha-…`; PRs smoke-build only |
| Platforms | `linux/amd64` + `linux/arm64` |
| Auth | `DOCKER_HUB_USERNAME`, `DOCKER_HUB_ACCESS_TOKEN` (same as hytale) |

### Checklist

- [x] **Workflows** — `docker.yml` (build/smoke/push) + `dockerhub-description.yml` (sync `readme.md` → Hub)
- [x] **Docs** — readme pull/badge + personal-assistant compose note
- [ ] **Secrets** on `shotah/ai-gantry` GitHub repo: `DOCKER_HUB_USERNAME`, `DOCKER_HUB_ACCESS_TOKEN` (copy from hytale-server-container if still valid)
- [ ] **First push** — merge to `main` (creates Hub repo + `edge`) or tag `v*`; then run **dockerhub-description** workflow if Hub README is empty
- [ ] **Verify** — [hub.docker.com/r/shotah/ai-gantry](https://hub.docker.com/r/shotah/ai-gantry) shows tags + full README; `docker pull shotah/ai-gantry:edge`

### Out of scope

- Baking MCP tool binaries into this image (that’s Tim / a derived image)
- Replacing GoReleaser binary releases

---

## Nice-to-have (later)

- [x] Multimodal Telegram (inbound photo → vision request; outbound `SendPhoto`)
- [ ] Optional `embedding BLOB` behind the same `memory_recall` interface if FTS
      ever proves too weak at this scale
