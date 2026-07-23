# examples/

Operator templates and a **usable** personal-assistant skeleton.

| Path | What it is |
| --- | --- |
| [`persona/*.example.md`](persona/) | System-prompt templates (`gantry init` embeds these) |
| [`mcp.toml.example`](mcp.toml.example) / [`env.example`](env.example) | Embedded by `gantry init` |
| [`personal-assistant/`](personal-assistant/) | **appliance-style compose stack** — copy, fill `.env`, `docker compose up` |

Production local-agent (tools baked in, remote deploy, auth helpers) lives in
**[`../local-agent/`](../local-agent/)** — same repo, appliance folder next to the kernel.

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

## Path C — full local-agent (recommended for real use)

```bash
cd local-agent
make init    # edit .env
make build && make up
# remote: set DEPLOY_* then make remote-deploy
```

Walkthrough: **[local-agent/README.md](../local-agent/README.md)**. Same gantry contract
(`PERSONA_DIR`, `MCP_MANIFEST`, `DATA_DIR`); richer image.

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
