# ai-gantry 🏗️

<p align="center">
  <img src="assets/banner.svg" alt="ai-gantry — an AI harness for long-horizon planning" width="100%">
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
> That frame is an **AI harness**: the runtime around the model (loop, tools,
> memory, context) so the agent can **plan on a long horizon** — days and
> weeks, not a single chat turn.

> Make a local harness small enough to understand, efficient enough to run
> continuously, resilient enough for imperfect local models, and stateful
> enough for long-horizon planning: a useful personality and standing goals
> rather than a stateless chatbot every time context gets expensive.

**Run your own agent.** Pull a container, point it at a local model or paste an
API key, and chat from your phone. No dashboard in the thing you talk to.
Nothing listens on a port.

```text
container + persona + any OpenAI-compat LLM  →  outbound chat
```

Gemini or ChatGPT with a key. Ollama on the same machine. Chat, memory, and
reminders work with **zero extra tools** — add MCP binaries later if you want.

We spent the engineering budget on the **harness** — tool calling, MCP,
context economics, memory that outlives a session, and finishing turns on
small local models. The harness is the product. Console, metrics, and
fleet automation live one layer up in
**[gantree](https://github.com/shotah/gantree)** — the shipping yard — and
stay out of this binary. Completeness of the platform is not the goal.
Long-horizon planning is.

If it clicks, the same binary grows with you (persona files, inspectable
SQLite, optional tools). If you need a team workspace on day one, this is
the wrong repo — and that’s fine.

---

## Hello (Docker)

You need Docker, a chat bot, and a model.

1. A [Gemini API key](https://aistudio.google.com/apikey) **or** any
   OpenAI-compatible endpoint (Ollama, xAI, …).
2. A Telegram bot token from [@BotFather](https://t.me/BotFather) and your
   numeric user id (e.g. [@userinfobot](https://t.me/userinfobot)).
   Discord and Slack work too — same compose file, different env.
   The mouth we own is **[gantry-pendant](https://github.com/shotah/gantry-pendant)**
   (`CHANNEL=pendant`). Telegram stays the default.

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

Images: `shotah/ai-gantry:latest` (current `main`), `:edge` (`main`), `:0.x.y` (pin).
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
LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
LLM_API_KEY=...
LLM_MODEL=gemini-3.5-flash
```

### Other ways to run

| Path | When |
| --- | --- |
| **[examples/docker/](examples/docker/)** | Laptop / any Docker host *(start here)* |
| **[examples/native/](examples/native/)** | Linux + systemd + a local model |
| **[examples/hosting/gcp/](examples/hosting/gcp/)** · **[aws](examples/hosting/aws/)** | Small always-on VM |
| **[gantree](https://github.com/shotah/gantree)** | Console, metrics, grant tools, several agents |
| **[gantry-pendant](https://github.com/shotah/gantry-pendant)** | Phone chat we own (`CHANNEL=pendant`) |
| `make init && make run` | Hack on the binary (`CHANNEL=stdio`) |

Cookbook: **[examples/README.md](examples/README.md)**.
A full life-stack (persona + MCP + compose) lives in a consumer repo, not this harness.

---

## Chat is the console

Ops live in the same chat you already opened. No second UI in *this*
binary, no inbound port. Type `/help` anytime. Graphs, a board of
agents, MCP grants without SSH — that is
**[gantree](https://github.com/shotah/gantree)**. It never sits in a
chat turn.

| Command | What it does |
| --- | --- |
| `/status` `/perf` `/tokens` | Session bounds, trajectory (invocations / tools / batch), prompt size |
| `/tools` `/examples` `/engagement` `/spark` `/new` `/cancel` | Catalog, ideas, looking-after-you wakes, reset, abort |
| `/auth` | Headless MCP login — paste a code; no laptop callback |

Telegram is the default. Discord, Slack, and
**[pendant](https://github.com/shotah/gantry-pendant)** (our chat client) are
shipped (one `CHANNEL` per process). Headless OAuth: **[docs/auth.md](docs/auth.md)**.

### Two files, not a catalog

Long-horizon means the person is still there tomorrow. Most agents *feel*
like someone after a long chat, then `/new` wipes them.

| File | Who writes it |
| --- | --- |
| `PERSONA.md` | You — who it should be, who you are, harness-builtin policy |
| `SELF.md` | The agent — voice, jokes, rituals, a few north-star aims that survive `/new` (you can delete any line) |

MCP tools are **not** listed in `PERSONA.md`. They come from the live catalog
(`/tools`, this turn’s schemas, `[mcp prefixes]`). Keep `PERSONA.md` short
(2–4k characters, examples over rule dumps) or the middle of it gets ignored.
Progress logs and dated to-dos are memory / cron, not persona.

How to write one: **[docs/persona.md](docs/persona.md)**.
Horizon split: **[docs/persona.md](docs/persona.md#where-the-horizon-lives)**.
`SELF.md` drift: **[docs/troubleshooting.md](docs/troubleshooting.md#selfmd--personality-drift)**.

---

## Read next

| If you want… | Go here |
| --- | --- |
| What we actually built (honest inventory) | **[docs/features.md](docs/features.md)** |
| How to write `PERSONA.md` (tight, no MCP catalog, where the horizon lives) | **[docs/persona.md](docs/persona.md)** |
| How the harness is put together | **[docs/architecture.md](docs/architecture.md)** |
| Env, loop, memory, long-horizon contract | **[docs/design.md](docs/design.md)** |
| Wiring MCP tools | **[docs/mcp.md](docs/mcp.md)** |
| Why outbound-only / who it’s for | **[docs/positioning.md](docs/positioning.md)** |
| Console, metrics, or several agents | **[gantree](https://github.com/shotah/gantree)** |
| Chat from a phone we own | **[gantry-pendant](https://github.com/shotah/gantry-pendant)** |
| Security notes | **[docs/security.md](docs/security.md)** |

The harness is a small static Go binary. Tools are optional MCP processes.
We spent the budget on the loop so a **small local model** can finish a
tool turn instead of ERROR — that’s the production story, not a requirement
to start. Long-horizon planning is the reason the loop, memory, cron, and
`SELF.md` exist.

## License

MIT — see [LICENSE](LICENSE).
