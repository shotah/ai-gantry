# Gantree contract — harness work

Attack this file. Yard UI stays in [shotah/gantree](https://github.com/shotah/gantree).
Design note: [docs/gantree.md](docs/gantree.md). File/env/status JSON the
console may write and read: [docs/gantree-contract.md](docs/gantree-contract.md).

The harness never learns instance names. Gantree never sits in the token
path of a chat turn. No `/metrics` port. No dashboard hook on Completer
rounds, parallel tool calls, or RSS. `gantry status` is a **separate
process** (Docker healthcheck + operator exec): read-only files + SQLite
heartbeat + a boot snapshot. It must not dial MCP, load the LLM, or write.

Siblings: [todo.md](todo.md) (loop) · [apple_todo.md](apple_todo.md) ·
[aws_todo.md](aws_todo.md) · [gcp_todo.md](gcp_todo.md).

---

## Speed gate

If a change would make a Telegram turn slower so the yard looks nicer, it
is wrong. Snapshot `data/doctor.json` is written **once after MCP boot**,
never per tool call. Heartbeat Touch stays a one-row upsert. Chat-only
(empty `mcp.toml`) must keep Docker **healthy** — exit 0 is liveness,
not “tools are up.”

---

## Work

- [x] Richer `gantry status` / `doctor`: JSON on stdout — channel, each
      MCP connected vs skipped, auth declared yes/no, persona files present
- [x] Refuse operator `ok` when the manifest lists servers and all were
      skipped. Docker/healthcheck **exit code stays heartbeat** (alive)
- [x] Tool errors a model *and* a UI can tell apart: `no_binary` vs
      `no_key` vs `no_oauth` (stable `reason` codes + `tool error [code]:`)
- [x] Stable JSON slog fields for turns / tokens / recoveries
      (`turn perf`: `prompt_est_tokens`, `gen_est_tokens`, `iterations`,
      `recoveries`, …)
- [x] `user_id` + `session_id` on every `turn perf` line (needs an image
      bump before Mini cranes emit it; gantree pin may lag)
- [x] Stable file/env contract docs the console can write against
      ([docs/gantree-contract.md](docs/gantree-contract.md))

`gantry doctor` is an alias of `status`.
