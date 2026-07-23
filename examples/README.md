# examples/

Operator templates and a **usable** personal-assistant skeleton.

| Path | What it is |
| --- | --- |
| [`persona/*.example.md`](persona/) | System-prompt templates (`gantry init` embeds these) |
| [`mcp.toml.example`](mcp.toml.example) / [`env.example`](env.example) | Embedded by `gantry init` |
| [`personal-assistant/`](personal-assistant/) | **Tim-shaped compose stack** — copy, fill `.env`, `docker compose up` |

Production Tim (tools baked into the image, remote deploy, auth helpers) lives in
**[docker_open_claw](https://github.com/shotah/docker_open_claw)** — this repo is
the kernel; that repo is the appliance.

---

## Path A — local REPL (fastest)

```bash
make init          # → deploy/persona + deploy/mcp.toml + .env.example
cp .env.example .env
# set LLM_* ; CHANNEL=stdio is fine for make run
make run           # CHANNEL=stdio PERSONA_DIR=./deploy/persona
```

Slash commands: `/new` `/status` `/tools` `/quit`.

---

## Path B — Telegram bot (kernel image)

```bash
make example-pa    # seed examples/personal-assistant/persona + .env
# edit examples/personal-assistant/.env
#   GEMINI_API_KEY=...
#   TELEGRAM_BOT_TOKEN=...
#   TELEGRAM_ALLOWED_USERS=123456789

docker compose -f examples/personal-assistant/compose.yml up -d --build
docker compose -f examples/personal-assistant/compose.yml logs -f
```

Full walkthrough: **[personal-assistant/README.md](personal-assistant/README.md)**.

---

## Path C — full Tim (recommended for real use)

Clone the wrapper that bakes MCP binaries and ships `make remote-deploy`:

```bash
git clone https://github.com/shotah/docker_open_claw.git
cd docker_open_claw
make init && # edit .env
make build && make up
```

Same gantry contract (`PERSONA_DIR`, `MCP_MANIFEST`, `DATA_DIR`); richer image.

---

## Persona & MCP rules of thumb

- Concat order: `SOUL` → `IDENTITY` → `USER` → `AGENTS` → `TOOLS` → `HEARTBEAT` →
  `BOOTSTRAP` → `MEMORY` (then other `*.md`). Missing files are skipped.
- **Do not commit** filled-in personal `*.md` or `.env`.
- `mcp.toml`: listed server = may start. Prefer `--tool-tier core` / `tools = […]`
  so Flash is not fed huge schemas. Boot logs `tools_listed` vs `tools_published`.
- Data: `$DATA_DIR/gantry.db` holds sessions, memory, cron, heartbeat — survives
  image rebuilds if the volume mount stays.

Slash commands: `/new`, `/status`, `/tools`. Unix: `kill -HUP <pid>` reloads persona.
Telegram photos: inbound → vision; outbound `SendPhoto` for image URLs in replies.
