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

---

## vFun — Telegram message reactions

Reactions (❤️ 😢 👍 on a bot message) are invisible today: we only poll
`message` updates. Treat them as messaging — pipe through, no switch, no
emoji lists. Not reacting is the mute button; LLM/persona decide the reply.

| Choice | Pick |
| --- | --- |
| Inbound | Synthetic user line → full `agent.Handle` |
| Shape | `[reaction] 👍 on: <clip of target msg>` |
| Auth | Same allowlist as messages |

### Checklist

- [x] `AllowedUpdates` += `message_reaction`; parse `MessageReactionUpdated`
- [x] Cache recent outbound `message_id → text` for the clip
- [x] Allowlist → synthetic inbound → `agent.Handle`
- [x] Ignore bot/self; settle ~3s (overwrite latest emoji; clear cancels)
- [x] Tests + docs (`local-agent/docs/telegram.md`)
- [ ] (Later) Discord / Slack — same pipe-through

### Non-goals

- Feature flags / reply allowlists / emoji→category mapping
- Group-chat vote tallies (`message_reaction_count`)
- Pairing or open-inbox via reactions

---

## vFun — Telegram rich inbound (tagged)

Pipe low-friction Telegram payloads into the agent as tagged text (same path as
messages). Skip video/GIF/voice-without-STT.

- [x] Location / venue → `[location]` / `[venue]`
- [x] Contact → `[contact]`
- [x] Document metadata → `[document]` (no PDF extract yet)
- [x] Sticker emoji → `[sticker]`
- [x] Forward + reply-to → `[forwarded from …]` / `[reply to] …`
- [x] Tests + docs
- [ ] (Later) voice → STT; live-location `edited_message`; text/* document body

---

## Local Ubuntu + local model (binary / systemd)

**Two install paths — pick one at setup** (net-new; no migration tooling):

| Path | How it runs |
| --- | --- |
| **Docker** (existing) | compose / `make remote-deploy` |
| **Local** (this section) | binary + systemd + local LLM |

Same gantry binary and env contract either way. Docs describe both; choose one.

### Library: almost nothing to change

gantry already talks to **any OpenAI-compatible** endpoint via `LLM_BASE_URL` /
`LLM_API_KEY` / `LLM_MODEL` (Ollama = `http://127.0.0.1:11434/v1`). Memory is
SQLite + FTS5 — **no embeddings**, no vector service.

| Piece | Local work |
| --- | --- |
| Provider / agent loop | None — point `LLM_*` at Ollama |
| Embeddings | None — not used |
| MCP tools / Telegram | None — same as Docker |
| Repo deliverables | Docs + `gantry.service` + env example (packaging only) |

Optional later (quality, not blockers): trim `mcp.toml` if 35B struggles with
the full tool set; confirm Ollama/Qwen tool-calling quirks in practice.

**Hardware (personal):** [SER10 MAX](https://www.bee-link.com/products/beelink-ser10-max-amd-pro-ryzen-ai-9-hx-470-openclaw)
96GB — clean Ubuntu Server (wipe OEM/OpenClaw). **Model:**
[Qwen3.6-35B-A3B](https://ollama.com/library/qwen3.6:35b-a3b) Q4 via Ollama (`qwen3.6:35b-a3b`).

### Host prep (clean Ubuntu — before copying gantry)

Do this on the box **before** scp’ing the binary / env / persona:

```bash
# 1) Refresh apt sources + bring the system current (do this first)
sudo apt update
sudo apt upgrade -y

# 2) Basics for HTTPS / curl installers / clocks
sudo apt install -y ca-certificates curl tzdata

# 3) Install Ollama first (creates ollama.service), then enable + pull
# https://ollama.com/download/linux
curl -fsSL https://ollama.com/install.sh | sh
sudo systemctl enable --now ollama
# Ollama library uses name:tag — NOT HuggingFace "Qwen/Qwen3.6-35B-A3B"
# https://ollama.com/library/qwen3.6
ollama pull qwen3.6:35b-a3b


# 4) Runtime user + empty tree
sudo useradd --system --home /opt/gantry --shell /usr/sbin/nologin gantry
sudo mkdir -p /opt/gantry/{data,persona}
sudo chown -R gantry:gantry /opt/gantry
```

**Acceleration is the point of this box** — get the **Radeon 890M iGPU** into
Ollama’s path (ROCm and/or Vulkan). That is *not* the same as dumping
[Beelink’s SER10 Driver folder](https://dr.bee-link.cn/?dir=uploads%2FSER%2FSER10%2FDriver)
onto Ubuntu. That portal is mostly **Windows** chipset/WiFi/audio packs; on a
clean Ubuntu Server install, prefer kernel `amdgpu` + current Ollama AMD docs.
Use Beelink ZIPs only if something basic is missing (e.g. WiFi) *and* they
actually ship a Linux package — don’t install Windows `.exe`/`.inf` on Linux.

**Linux GPU path (document what works on HX 470 / 890M):**

1. Fresh kernel after `apt upgrade` (reboot). Prefer a recent Ubuntu/HWE kernel;
   Strix-class iGPU + dynamic VRAM has bitten people on older kernels.
2. BIOS: if Ollama won’t see the iGPU / tiny VRAM, try **fixed UMA frame buffer**
   (e.g. large fixed size) instead of Auto — common fix on Ryzen AI 300 / 890M.
3. Install latest Ollama; enable iGPU if logs say it was dropped, e.g.
   `Environment=OLLAMA_IGPU_ENABLE=1` on `ollama.service` (see Ollama issues for
   890M / gfx1150).
4. Confirm GPU in `ollama ps` / logs (`library=ROCm` or `Vulkan`, not 100% CPU).
5. **NPU (XDNA / “86 TOPS”)** is a separate stack — nice for other AI apps;
   Ollama chat today is iGPU/CPU. Don’t block on NPU drivers for gantry.

CPU+96GB remains a valid fallback while GPU accel is being sorted.

**Not required for gantry itself:** Docker, Go, Python, sqlite apt, embeddings.

**MCP tools (optional, later):** same static Go binaries as the Docker image
(`google-workspace-mcp-go`, `strava-mcp`, `garmin`, `mcp-gemini-google-search`,
`mcp-beam`, `youtube-go-mcp`) on `PATH` + secrets. Search MCP still needs a
Gemini key. Fine to start with an empty / minimal `mcp.toml`.

### Local layout (one tree)

```text
/opt/gantry/
  gantry            # binary
  gantry.env        # LLM_* → :11434/v1, Telegram, DATA_DIR=…
  mcp.toml
  persona/
  data/             # gantry.db
```

systemd unit in repo → `/etc/systemd/system/gantry.service`
(`WorkingDirectory=/opt/gantry`, `EnvironmentFile=…/gantry.env`,
`Restart=always`, `After=`/`Requires=` `ollama.service`).

### Checklist

- [x] Clean Ubuntu Server (wipe OEM/OpenClaw)
- [x] Host prep: `apt update` + `apt upgrade`, ca-certs/curl/tzdata, user + `/opt/gantry`
- [x] **iGPU accel (tim / 192.168.1.39):** `OLLAMA_IGPU_ENABLE=1` → logs
      `inference compute ... library=ROCm ... gfx1150 ... type=iGPU`; `ollama ps` →
      `100% GPU`. Dedicated agent box (outbound OK; his to use).
- [x] Pull Qwen3.5-35B via Ollama; iGPU confirmed (`100% GPU`)
- [x] Docs: Docker vs local install — pick one (`local-agent/deploy/README.md`)
- [x] Ship unit + `gantry.env` example under `local-agent/deploy/`
- [x] `make remote-native-deploy` (fetch/sync/install/systemd) — like compose remote
- [x] Memory/persona/secrets staged on `/opt/gantry` (scp migrate)
- [ ] Cutover: stop Docker TIM → `make remote-native-deploy` → smoke chat + tools

### Non-goals

- Library provider rewrite / embeddings for local
- Docker↔local migration tooling
- Beelink/OpenClaw preinstalled OS
- Treating Beelink’s Windows Driver ZIP as the Linux ROCm install
- apt-installing MCP stacks (use static binaries like the image)
- Blocking on NPU/XDNA for gantry chat
