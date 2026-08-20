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

**Run your own agent.** Pull a container, point it at a local model or paste an
API key, and chat from your phone. No dashboard. No config UI. Nothing listens
on a port.

```text
container + persona + any OpenAI-compat LLM  →  outbound chat
```

Gemini or Grok with a key. Ollama on the same machine. Chat, memory, and
reminders work with **zero extra tools** — add MCP binaries later if you want.

This is for someone who wants to try a personal agent they actually own, not
for someone who wants a multi-agent platform. If it clicks, the same binary
grows with you (persona files, inspectable SQLite, optional tools). If you
need a web UI or a team workspace, this is the wrong repo — and that’s fine.

---

## Hello (Docker)

You need Docker, a chat bot, and a model.

1. A [Gemini API key](https://aistudio.google.com/apikey) **or** any
   OpenAI-compatible endpoint (Ollama, xAI, …).
2. A Telegram bot token from [@BotFather](https://t.me/BotFather) and your
   numeric user id (e.g. [@userinfobot](https://t.me/userinfobot)).
   Discord and Slack work too — same compose file, different env.

```bash
docker pull shotah/ai-gantry:latest
docker run --rm shotah/ai-gantry:latest version

git clone https://github.com/shotah/ai-gantry.git && cd ai-gantry
make example-docker
cd examples/docker
# set in .env:
#   GEMINI_API_KEY=...          # or LLM_BASE_URL + LLM_API_KEY + LLM_MODEL
#   TELEGRAM_BOT_TOKEN=...
#   TELEGRAM_ALLOWED_USERS=...  # numeric id, allowlist only
make up
make logs
```

Message the bot, then send `/status`. Walkthrough:
**[docs/deploy-docker.md](docs/deploy-docker.md)**.

Images: `shotah/ai-gantry:latest` (release), `:edge` (`main`), `:0.x.y` (pin).
Same tags on `ghcr.io/shotah/ai-gantry`.

### Drop in a different model

The agent does not care which vendor you picked. Set these three and restart:

| You have | Set |
| --- | --- |
| Gemini key | `GEMINI_API_KEY` (the compose file maps it) |
| Another cloud API | `LLM_BASE_URL` + `LLM_API_KEY` + `LLM_MODEL` |
| Ollama on Linux | **[examples/native/](examples/native/)** · [deploy-native](docs/deploy-native.md) |

```bash
# examples/docker/.env — xAI, for instance
LLM_BASE_URL=https://api.x.ai/v1
LLM_API_KEY=...
LLM_MODEL=grok-4
```

### Other ways to run

| Path | When |
| --- | --- |
| **[examples/docker/](examples/docker/)** | Laptop / any Docker host *(start here)* |
| **[examples/native/](examples/native/)** | Linux + systemd + a local model |
| **[examples/hosting/gcp/](examples/hosting/gcp/)** · **[aws](examples/hosting/aws/)** | Small always-on VM |
| `make init && make run` | Hack on the binary (`CHANNEL=stdio`) |

Cookbook: **[examples/README.md](examples/README.md)**.
A full life-stack (persona + MCP + compose) lives in a consumer repo, not this kernel.

---

## Chat is the console

Ops live in the same chat you already opened. No second UI, no inbound port.
Type `/help` anytime.

| Command | What it does |
| --- | --- |
| `/status` `/perf` `/tokens` | Session bounds, last-turn timing, prompt size |
| `/tools` `/examples` `/new` `/cancel` | Catalog, ideas, reset session, abort a turn |
| `/auth` | Headless MCP login — paste a code; no laptop callback |

Telegram is the default. Discord and Slack are shipped (one `CHANNEL` per
process). Headless OAuth: **[docs/auth.md](docs/auth.md)**.

### Personality that survives `/new`

Most agents *feel* like someone after a long chat, then `/new` wipes them.
Gantry writes voice, jokes, and rituals into `SELF.md` so they last — and you
can delete any line you don’t like. Details:
**[docs/troubleshooting.md](docs/troubleshooting.md#selfmd--personality-drift)**.

---

## Read next

| If you want… | Go here |
| --- | --- |
| What we actually built (honest inventory) | **[docs/features.md](docs/features.md)** |
| How the process is put together | **[docs/architecture.md](docs/architecture.md)** |
| Env, memory, agent loop, packaging | **[docs/design.md](docs/design.md)** |
| Wiring MCP tools | **[docs/mcp.md](docs/mcp.md)** |
| Why outbound-only / who it’s for | **[docs/positioning.md](docs/positioning.md)** |
| Security notes | **[docs/security.md](docs/security.md)** |

The kernel is a small static Go binary. Tools are optional MCP processes. The
loop is built so a **small local model** can finish a tool turn instead of
erroring — that’s the production story, not a requirement to start.

## License

MIT — see [LICENSE](LICENSE).
