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

---

## Slog what Completer already returned

No second HTTP call. No provider usage API. No `/metrics`. Copy fields
off the Completer response you already have onto the existing
`turn perf` line. If it would add latency to a Telegram turn, it is
wrong.

Gantree parses these keys. Native-token tiles fill when `usage` was on
the Completer response; chars/4 stays the fallback. Spend `unknown` is
only a missing/invalid `source` (legacy logs before this line), not a
guess.

- [x] **`source` is always on `turn perf`.** `user` / `cron` / `watch` /
      `reaction` — that set only (Gantree charts anything else as
      **unknown**). Spark / examples wake as `cron`. Cron may omit
      `user_id`. A user turn with a channel id falls back to `chat_id`
      and must not omit `user_id`. Empty `source` is never logged.
- [x] **Native OpenAI-compat `usage` on the same line** when the
      Completer response had it: `prompt_tokens`, `completion_tokens`,
      `total_tokens` (summed across rounds). Details when the provider
      sent them: `cached_tokens`, `cache_write_tokens`,
      `reasoning_tokens`, audio / prediction counts. Keep
      `prompt_est_tokens` / `gen_est_tokens` as the chars/4 fallback.
      Streaming requests `stream_options.include_usage` so the trailing
      usage chunk is not dropped (no extra HTTP call).
- [x] **`model` + `finish_reason`** on that line when the response
      had them (`model` falls back to `LLM_MODEL`). `service_tier` and
      `duration_ms` (alias of `total_ms`) go with them.
