# Milestone archive (build order)

Historical checklist for how ai-gantry was built (M0–M7). All shipped;
Tim/ZeroClaw cutover is done. Open follow-ups live in [todo.md](../todo.md).

---

## Milestone 0 — scaffold

- [x] New repo `ai-gantry`, Go module, `cmd/gantry` + `internal/` skeleton
- [x] `golangci-lint` config, CI (vet/lint/test), MIT/Apache license
- [x] `Dockerfile` (multi-stage, distroless/static final, CGO off), `compose.yml` sample
- [x] `internal/config`: env struct, fail-fast validation, unit tests

## Milestone 1 — talk (no tools)

- [x] `internal/provider`: OpenAI-compat chat client (base URL/key/model), streaming optional
- [x] `internal/persona`: load + concat `/persona/*.md` (fixed order, missing-file tolerant)
- [x] `internal/channel/stdio`: dev REPL channel
- [x] `internal/agent`: minimal loop (prompt → model → reply), no tools yet
- [x] Milestone test: chat with persona via `docker run -it`

## Milestone 2 — Telegram

- [x] `internal/channel/telegram`: long-poll, allowlist, typing indicator, message splitting (4096 chars)
- [x] `/new` and `/status` commands
- [x] `internal/session`: SQLite-backed bounded history, token estimate, trim rules
- [x] Milestone test: daily-drivable chat bot in a container

## Milestone 3 — MCP host (the point of the project)

- [x] `internal/mcp`: manifest parse, spawn via official go-sdk, eager tool listing
- [x] Tool call execution + per-result truncation + iteration cap
- [x] Supervisor: restart with backoff, stderr → slog, boot-time hard fail if a manifest server can't start
- [x] Tool-name collision handling (prefix with server name as `{server}__{tool}`)
- [x] Live deploy check: google-workspace-mcp-go + other MCP binaries from Telegram
  (unit/in-memory SDK covered in-repo; live binaries are compose/image work)

## Milestone 4 — memory

- [x] `Memory` interface (store/recall/forget + hydrate) so backends are swappable
- [x] `internal/memory`: builtin backend — schema, WAL, FTS5, migrations (embedded SQL)
- [x] `MEMORY_BACKEND=mcp:<name>` adapter: route the three tools + hydration to a manifest server
- [x] Built-in tools: `memory_store` / `memory_recall` / `memory_forget`
- [x] Hydration block in prompt assembly (cap ~30 rows, persona precedence rule in system prompt)
- [x] Consolidation timer job (cheap pass, bounded batch, `0` disables)
- [x] `sqlite3`-friendly docs: how to inspect/fix memory by hand ([memory.md](memory.md))
- [x] Milestone test: store→recall across `/new`; consolidation dedupes; `memory_forget` works

## Milestone 5 — hardening & cutover

- [x] `gantry status` healthcheck + heartbeat row
- [x] Rolling session summary (context compression v2)
- [x] Graceful shutdown (finish in-flight turn, kill MCP children)
- [x] Load test: fat tool dumps don't blow context (assert bounded prompt size)
- [x] Side-by-side deploy next to ZeroClaw Tim; cut over when trusted
- [x] Retire ZeroClaw Tim service; pin gantry release tags

## Milestone 6 — cron / scheduled turns

Proactive jobs are **kernel work**: fire time → run the normal agent loop
(tools/MCP allowed) → **push the reply on Telegram** (no inbound user message).

- [x] `internal/cron`: SQLite `job` rows (one-shot + simple schedules), timezone via env (`CRON_TZ`)
- [x] Builtin tools: `cron_schedule` / `cron_list` / `cron_cancel`
- [x] Ticker/wake loop: due jobs → synthetic user prompt → `agent.Handle` → channel **push**
- [x] Channel outbound: Telegram `SendMessage` / stdio print; delivery metadata from scheduling turn
- [x] Caps: max active jobs, skip/overlap policy if a run is still in flight
- [x] Docs: [cron.md](cron.md)
- [x] Milestone test: schedule → fire → channel receives agent reply; cancel works

## Milestone 7 — streaming replies to Telegram

Streaming **to the user** is channel-layer work (edit the Telegram message as
model tokens arrive). Distinct from MCP servers streaming tool results into the
gantry.

- [x] Provider: streaming `CompleteStream` (OpenAI-compat SSE / chunk API)
- [x] Agent: stream final-text path (tool-call chunks skip onText; tools still work)
- [x] Telegram: send placeholder → `editMessageText` throttle (rate gap + 4096 clip)
- [x] Stdio: optional token stream to stdout (keep JSON logs on stderr)
- [x] Config knob (`STREAM_REPLIES=false` default until you opt in)
- [x] Milestone test: stream deltas to ReplyWriter; Telegram finish falls back to send on edit failure
