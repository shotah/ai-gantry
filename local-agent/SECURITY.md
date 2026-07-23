# Security Policy

## Reporting a Vulnerability

For **this wrapper repo**, open a private security advisory or contact the maintainer.

For **ai-gantry itself** (the runtime), follow upstream policy: [shotah/ai-gantry security policy](https://github.com/shotah/ai-gantry/security/policy). Do not file public issues for runtime vulns here.

## Hardening defaults in this stack

- Telegram allowlist (`TELEGRAM_ALLOWED_USERS`) is the entire auth model — required; an empty allowlist fails boot
- **No inbound ports, ever** — Telegram long-polls outbound; the healthcheck is an exit code, and there is no gateway or dashboard
- `mcp.toml` is the entire tool grant — a server not listed does not exist to the model
- Runtime image is distroless/static: no shell, no libc, nothing for a compromised tool to shell out to
- Secrets in `.env` / `secrets/*` only — never commit
- Treat `./data` (`gantry.db`: sessions + memory) and `persona/*.md` as sensitive

Deeper runtime tradeoffs: [ai-gantry docs/security.md](https://github.com/shotah/ai-gantry/blob/main/docs/security.md).
