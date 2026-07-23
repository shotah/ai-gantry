# Personal assistant example (Tim-shaped)

A **copy-paste deploy skeleton** modeled on the production Tim wrapper
([docker_open_claw](https://github.com/shotah/docker_open_claw)): Gemini chat,
Telegram allowlist, persona mounts, SQLite under `./data`, and an `mcp.toml`
shaped like a real tool kit.

This folder uses the **kernel-only** `gantry` image from this repo (no MCP
binaries baked in). Chat + memory + cron work out of the box. To grant tools,
either:

1. **Full Tim image** — clone/use [docker_open_claw](https://github.com/shotah/docker_open_claw)
   (gantry + Workspace / Strava / Garmin / Cast / YT Music / search baked in), or
2. **Your own image** — `FROM` distroless/static, `COPY` static MCP binaries onto
   `PATH`, uncomment the matching `[[server]]` blocks in `mcp.toml`.

## Layout

```text
personal-assistant/
  compose.yml      # one service, no ports, exec-form healthcheck
  .env.example     # GEMINI_* + TELEGRAM_* (mapped to LLM_* in compose)
  mcp.toml         # Tim-shaped servers (commented until binaries exist)
  persona/         # seeded from ../persona/*.example.md (see setup)
  data/            # created at runtime → gantry.db
```

## Setup (5 minutes)

From the **ai-gantry repo root**:

```bash
make example-pa    # seeds persona/*.md + .env (skips existing)
# Edit examples/personal-assistant/.env:
#   GEMINI_API_KEY=...
#   TELEGRAM_BOT_TOKEN=...
#   TELEGRAM_ALLOWED_USERS=123456789

docker compose -f examples/personal-assistant/compose.yml up -d --build
docker compose -f examples/personal-assistant/compose.yml logs -f
```

Message the bot. First checks:

| You send | Expect |
| --- | --- |
| `hi` | Short reply in character |
| `/status` | uptime, model, history, tool count |
| `/tools` | builtin memory/cron tools (MCP empty until you add servers) |
| `/new` | `session reset` |

### Local REPL instead of Telegram

In `.env` set `CHANNEL=stdio`, then:

```bash
docker compose -f examples/personal-assistant/compose.yml run --rm -it gantry
```

Or from repo root without Docker: `make init && make run` (uses `deploy/`).

## Adding MCP tools (like Tim)

1. Put static binaries on `PATH` inside the image (see Tim's
   [Dockerfile](https://github.com/shotah/docker_open_claw/blob/main/Dockerfile)).
2. Uncomment the matching `[[server]]` in `mcp.toml`.
3. Mount secrets the child expects (OAuth tokens, Garmin session, etc.) — Tim's
   compose shows the bind-mount pattern under `./secrets/…` → `/data/.config/…`.
4. Prefer MCP-native filters (`--tool-tier core`) **and/or** gantry `tools = […]`
   so Flash is not fed 100+ schemas.

Boot log should show `tools_listed` / `tools_published` per server and a
`tool schema estimate`.

## What survives rebuilds

| Path | Persists? |
| --- | --- |
| `./data/gantry.db` | **Yes** — sessions, memory, cron, heartbeat |
| `./persona/*.md` | Yes (your bind mount) |
| `./mcp.toml` | Yes (your bind mount) |
| Container image | Disposable — rebuild anytime |

## Production Tim

For the full assistant (remote deploy, auth helpers, tool pins), use the wrapper
repo — this example is the **kernel contract** made concrete:

→ **[shotah/docker_open_claw](https://github.com/shotah/docker_open_claw)** (`make init && make up`)
