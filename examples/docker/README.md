# gantry-compose

Template **consumer** repository for [ai-gantry](https://github.com/shotah/ai-gantry).
Pulls the published kernel image and runs it with Compose — persona, `mcp.toml`,
and SQLite data stay in this repo. No kernel source tree required.

| Consumes | [`shotah/ai-gantry`](https://hub.docker.com/r/shotah/ai-gantry) (`:latest` / `:edge` / `:0.x.y`) |
| Channel | Telegram by default (Discord / Slack / stdio via `.env`) |
| LLM | Gemini (or any OpenAI-compat endpoint) |

Upstream docs: [deploy-docker](https://github.com/shotah/ai-gantry/blob/main/docs/deploy-docker.md) ·
[discord](https://github.com/shotah/ai-gantry/blob/main/docs/discord.md) ·
[slack](https://github.com/shotah/ai-gantry/blob/main/docs/slack.md).

Sibling templates: [native (systemd)](../native/) · [hosting (GCP · AWS)](../hosting/).

## Layout

```text
.
  compose.yml          # one service, no ports, exec-form healthcheck
  .env.example         # GEMINI_* + TELEGRAM_* → LLM_* in compose
  mcp.toml             # MCP servers (commented until binaries exist)
  persona/*.example.md # system prompt templates
  data/                # runtime → gantry.db
  Makefile             # init / up / logs / status
```

## Quick start

```bash
make init
# Set in .env:
#   GEMINI_API_KEY=...
#   TELEGRAM_BOT_TOKEN=...
#   TELEGRAM_ALLOWED_USERS=<numeric id>

make up
make logs
```

Chat + memory + cron work with zero MCP servers. First checks:

| Send | Expect |
| --- | --- |
| `hi` | Short reply in character |
| `/status` | uptime, model, history, tool count |
| `/tools` | builtin memory/cron + `self_note` (MCP empty until servers are granted) |
| `/new` | `session reset` (or `… — personality distilled into SELF.md` after a real chat) |

### Discord or Slack

Same Compose stack; set channel + tokens in `.env`:

```bash
CHANNEL=discord
DISCORD_BOT_TOKEN=...
DISCORD_ALLOWED_USERS=123456789012345678
```

Then `make up`.

### Local REPL

`CHANNEL=stdio` in `.env`, then:

```bash
docker compose run --rm -it gantry
```

## MCP tools

1. Bake or mount static MCP binaries on `PATH` (`FROM shotah/ai-gantry` + `COPY`).
2. Uncomment the matching `[[server]]` in `mcp.toml`.
3. Mount secrets under `./secrets/…` → `/data/.config/…` as needed.
4. Prefer MCP `--tool-tier` / gantry `tools = […]` so Flash is not fed huge schemas.

Full appliance with tools pre-baked:
[ai-gantry/local-agent](https://github.com/shotah/ai-gantry/tree/main/local-agent).

## Persistence

| Path | Survives image pulls? |
| --- | --- |
| `./data/gantry.db` | Yes |
| `./persona/*.md` | Yes (incl. agent-written `SELF.md`) |
| `./mcp.toml` / `.env` | Yes |
| Container image | Disposable |

### Persona write access (`SELF.md`)

`compose.yml` mounts `./persona` **writable** so the agent can keep
`SELF.md` (voice / jokes / rituals) via the `self_note` tool and a distill
pass on `/new`. Only that one file is agent-written; the rest are still
yours to edit. Mounting `:ro` (or a read-only host dir) makes self-notes
auto-disable at boot — check logs for `self-notes disabled`. Set
`SELF_NOTES_ENABLED=false` if you want a read-only persona on purpose.

## Publishing this template

Copy this directory into a new git remote (or use it as a GitHub template).
Pin `image:` to a release tag for production; use `:edge` only for tracking upstream `main`.
