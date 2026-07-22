# ai-gantry — TODO

Tracked work from Tim / `docker_open_claw` integration. Status: open unless
marked done.

---

## Scaffold & examples

- [x] **`gantry init`** — embeds `examples/` templates; scaffolds `PERSONA_DIR`,
      `MCP_MANIFEST`, `.env.example` (skip existing; fail-fast if not writable).
- [x] **`examples/` is the source of truth** — `deploy/` regenerated via
      `make init` / `gantry init`.
- [x] **`make init`** + docs quick-start wired to `gantry init`.

## Tool surface (context budget)

- [x] **Per-server `tools` / `exclude` / `tools_prefix` in `mcp.toml`** — host
      filters after `tools/list`; boot logs listed vs published counts.
- [x] **`TOOL_SCHEMA_MAX_TOKENS`** — log `est_tokens` (chars/4); `>0` hard-fails.
- [ ] **Sibling: go-garmin `--tools` / tier filter** — prefer curated defaults in
      [shotah/go-garmin](https://github.com/shotah/go-garmin); gantry allowlist
      is belt-and-suspenders. *(tracked upstream, not in this repo)*

## Docs / cutover

- [x] **Tim cutover** — README §10 expanded (`DEPLOY_PATH`, second bot token,
      memory non-migration, no gws/busybox/gateway).
- [x] **`examples/README.md`** — operator entry for mounts + filters + init.

## Nice-to-have

- [x] Optional `tools_prefix` override per server.
- [x] `/tools` command (stdio + Telegram via agent).
- [x] Persona reload on SIGHUP (unix).
- [x] CI/test gate: `examples/` `*.example.md` matches `persona.PreferredOrder`.

---

## Done (context)

- [x] Milestone 0–7 (scaffold → Telegram → MCP → memory → hardening → cron → stream)
- [x] Distroless/static, env + mounts config plane, allowlist-only Telegram
- [x] Coverage badge → `gh-pages` / README
