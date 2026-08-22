# Deploy: Docker (Hub / compose)

Run the **AI harness** as a **distroless container**: outbound chat only,
mounts for persona + `mcp.toml` + data. Fastest path when you want a cloud
OpenAI-compatible LLM (Gemini is the cookbook default) without installing a
local model host. Long-horizon pieces (memory, cron, `SELF.md`) are in this
image; MCP tools are optional extras you grant.

**Published images** (CI on every `main` push and `v*` tag):

| Registry | Image |
| --- | --- |
| [Docker Hub](https://hub.docker.com/r/shotah/ai-gantry) | `shotah/ai-gantry:latest` / `:edge` / `:0.x.y` |
| GHCR | `ghcr.io/shotah/ai-gantry:…` (same tags) |

`:latest` / `:edge` = `main` · pin `:0.x.y` for production.

Harness contract (env, mounts, MCP): [design.md](design.md). Hello path:
[root readme](../readme.md). Why Hub is the stranger path:
[positioning.md](positioning.md). Tool naming: [mcp.md](mcp.md).
A full life-stack (persona + MCP + compose) lives in a consumer repo, not this harness.

```mermaid
flowchart LR
  TG[Telegram / Discord / Slack] <-->|outbound only| C
  subgraph C["container — distroless/static"]
    G[gantry]
    M[MCP binaries]
    G -->|stdio| M
  end
  G -->|HTTPS OpenAI-compat| LLM[Gemini / ChatGPT / …]
```

---

## Hello in five minutes (pull from Hub, no tools)

**Telegram + Gemini.** You need: Docker, a
[Gemini API key](https://aistudio.google.com/apikey), a bot token from
[@BotFather](https://t.me/BotFather), and your numeric user id (e.g.
[@userinfobot](https://t.me/userinfobot)).

```bash
docker pull shotah/ai-gantry:latest
docker run --rm shotah/ai-gantry:latest version

git clone https://github.com/shotah/ai-gantry.git && cd ai-gantry
# Consumer template (copy examples/docker to a new repo, or seed in-tree):
make example-docker
cd examples/docker
# set GEMINI_API_KEY, TELEGRAM_BOT_TOKEN, TELEGRAM_ALLOWED_USERS in .env
make up
make logs
```

Message the bot → `/status` → `/new`. Memory and cron work immediately; MCP
servers stay commented until tools are granted.

Always-on VM templates: **[examples/hosting/](../examples/hosting/)**
([GCP GCE](../examples/hosting/gcp/) · [AWS EC2](../examples/hosting/aws/)).

### Discord / Slack (same compose)

Swap the channel in `.env` after [discord.md](discord.md) or [slack.md](slack.md):

```bash
CHANNEL=discord
DISCORD_BOT_TOKEN=...
DISCORD_ALLOWED_USERS=123456789012345678
```

Then `make up` from the consumer tree.

---

## Compose contract

```yaml
services:
  gantry:
    image: shotah/ai-gantry:latest   # or :edge / :0.x.y
    env_file: .env
    volumes:
      # Persona writable for SELF.md (self_note + /new distill). Use :ro only with SELF_NOTES_ENABLED=false.
      - ./deploy/persona:/persona
      - ./deploy/mcp.toml:/etc/gantry/mcp.toml:ro
      - ./deploy/data:/data        # gantry.db (sessions + memory)
      - ./deploy/secrets:/secrets:ro
    healthcheck:
      # exec form + full path — distroless has no shell
      test: ["CMD", "/usr/local/bin/gantry", "status"]
```

Second persona / LLM = second service block. Nothing inbound; health is
`gantry status` (exit code).

---

## MCP tool auth (browser OAuth)

Chat works with **zero** MCP servers. When you grant tools that need a browser
login (Google Workspace, Strava, …), authorize **once**.

**Headless / remote (preferred on a server):** chat `/auth <server>` — PKCE
paste or device flow, no inbound ports. Tokens land on the box running gantry.
See **[auth.md](auth.md)**.

**Laptop with a browser:** run the harness CLI on the machine that will receive
the localhost callback:

```bash
gantry auth google      # browser → http://localhost:4100/…
gantry auth ghealth     # browser → http://127.0.0.1:4101/…
gantry auth strava      # browser → http://localhost:19876/…
gantry auth youtube     # device flow (no localhost callback)
gantry auth garmin      # TTY login, or chat MFA with GARMIN_EMAIL/PASSWORD
```

What to expect:

1. A URL prints in the terminal (a Distroless container cannot open a browser).
2. Open it, approve access.
3. The provider redirects to `http://localhost:<port>/…` on **this** machine.
4. Tokens land under `DATA_DIR` (typically `data/.config/…`). Copy them to the
   server if gantry runs elsewhere.

**Do not** run Google/Strava loopback auth over SSH on a headless box — the
callback is `localhost` on *your* PC. Use chat `/auth` instead.

---

## When to prefer Docker

| Prefer Docker / Hub when… | Prefer [native](deploy-native.md) when… |
| --- | --- |
| You want `docker pull` + compose in minutes | You want Ollama/Qwen on metal (no container tax) |
| Cloud LLM (Gemini/ChatGPT) is fine | Local model + systemd on a mini-PC |
| Distroless sandbox is the grant story | Host PATH + `/opt/gantry` tree is enough |

Model swap is always `LLM_BASE_URL` + `LLM_API_KEY` + `LLM_MODEL` — same
binary in either supervisor.
