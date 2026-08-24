# Console contract (gantree)

What a yard console may **write** and **read** against this harness.
Gantree is the consumer: [shotah/gantree](https://github.com/shotah/gantree).
Harness-side design: [gantree.md](gantree.md). Work list: [../gantree_todo.md](../gantree_todo.md).

The harness never learns instance names. The console never sits in a chat
turn. If a field is not on this page, do not invent a hook for it.

---

## Speed

`gantry status` is a **separate process**. Docker healthchecks and
`docker exec … gantry status` must stay cheap:

- read-only SQLite heartbeat (`mode=ro`)
- `stat` persona files, parse `mcp.toml`, read `data/doctor.json`
- **never** dial MCP, never load LLM env, never write

`data/doctor.json` is written **once** after MCP boot, not per tool call.
Heartbeat Touch stays a one-row upsert on its existing interval.

Exit 0 is **liveness** (heartbeat fresh). Chat with an empty manifest, or
a listed manifest that all skipped, must not restart the container.

---

## Files the console may write

Same three mounts as [design.md](design.md). Recreate the container after
env or `mcp.toml` changes (restart keeps a ghost allowlist).

| Path (in the container) | Env | Who writes | Notes |
| --- | --- | --- | --- |
| `/persona/PERSONA.md` | `PERSONA_DIR` | operator / console | Required standing identity. Missing is tolerated at boot (empty persona). |
| `/persona/SELF.md` | `PERSONA_DIR` | the agent | Console may show / prune; do not invent MCP catalogs here. |
| `/etc/gantry/mcp.toml` | `MCP_MANIFEST` | operator / console | `[[server]]` listed = granted. Omit = does not exist. |
| `/data/` | `DATA_DIR` | process + console | `gantry.db`, OAuth session files, `doctor.json`. Never copy between instances. |
| `.env` (compose) | process env | operator / console | Secrets. See `.env.example`. MCP children inherit this env. |

`mcp.toml` fields the console already writes: `name`, `command`, `args`,
`auth_args` / `auth_command`, `tools`, `exclude`, `tools_prefix`, `force`,
`download_url`, `download_tag`. Runtime ignores download_* (that's
`tools-fetch`).

OAuth session files are server-specific (often `/data/<name>-oauth.json`
or a path in `auth_args`). The harness does not invent those names.

---

## Env the console may write

Required for a talking crane (fail-fast at `gantry run`):

| Key | Role |
| --- | --- |
| `LLM_BASE_URL` | OpenAI-compat endpoint |
| `LLM_API_KEY` | Provider key |
| `LLM_MODEL` | Model id |
| `CHANNEL` | `telegram` (default) · `discord` · `slack` · `stdio` |
| Channel token + allowlist | See `.env.example` — empty allowlist fails boot |

Optional knobs (history, tools, memory, cron, watch, spark, stream) live
in `.env.example`. Do not add a settings API. MCP API keys (`GOOGLE_*`,
`X_BEARER_TOKEN`, …) are process env the child inherits — not gantry knobs.

`gantry status` reads only `DATA_DIR`, `PERSONA_DIR`, `MCP_MANIFEST`,
`CHANNEL` (with the same defaults as `run`). It does **not** call
`config.Load()`, so a healthcheck never needs the LLM key.

---

## `gantry status` JSON

Stdout is one JSON object. Stderr may add `status: <reason>` when not
alive. `gantry doctor` is an alias.

```json
{
  "alive": true,
  "ok": false,
  "reason": "mcp_all_skipped",
  "version": "0.1.70",
  "commit": "b2f7cfe",
  "channel": "telegram",
  "persona": { "dir": "/persona", "persona_md": true, "self_md": false },
  "mcp": {
    "listed": 2,
    "connected": 0,
    "skipped": 2,
    "servers": [
      {
        "name": "google",
        "state": "skipped",
        "reason": "no_oauth",
        "note": "oauth token missing",
        "auth": true
      }
    ]
  }
}
```

| Field | Meaning |
| --- | --- |
| `alive` | Heartbeat row fresh (≤ ~60s). **This** is Docker exit 0/1. |
| `ok` | Operator health. False when `alive` is false, or listed MCP are all skipped / none connected. Empty manifest (chat-only) stays `ok` if alive. |
| `reason` | `no_heartbeat` · `mcp_all_skipped` |
| `version` | Harness semver from the binary ldflags (`gantry version`). Present even when `alive` is false — same image, same stamp. Compose `:latest` is not this. |
| `commit` | Short git sha when the image was built (`none` omitted) |
| `mcp.servers[].state` | `connected` · `skipped` · `unknown` (no boot snapshot yet) |
| `mcp.servers[].reason` | `no_binary` · `no_key` · `no_oauth` · `connect` |
| `mcp.servers[].auth` | `mcp.toml` declared `auth_args` / `auth_command` |

Parse JSON. Do not regex the line for `ok` / `skipped` — `"ok":false`
contains the letters `ok`.

---

## Tool errors (model + UI)

Boot-skipped servers stay out of the prompt catalog (do not invite
invention). If the model still calls that prefix:

```text
tool error [no_oauth]: google is skipped (…) — do not invent google__* names
```

MCP `isError` results that classify as key/oauth/binary use the same
`tool error [<reason>]:` shape. Generic failures stay `tool error: …`.

---

## Slog the console may read

JSON on stderr. Turns:

`msg=turn perf` → `source` (`user` · `cron` · `watch` · `reaction`; **always
set** — other strings chart as spend unknown; spark / examples are `cron`),
`user_id` (required on `user` / `reaction`; cron/watch omit when empty),
`session_id`, `outcome`, `iterations`, `tool_calls`, `max_batch`, `recoveries`,
`tools_per_inv`, `prompt_est_tokens`, `gen_est_tokens` (chars/4 fallback),
`model_ms`, `tool_ms`, `total_ms`, `duration_ms` (same wall ms as `total_ms`),
`hydration_est_tokens`, `model`, `finish_reason`.

When the Completer response had OpenAI-compat `usage` (summed across rounds):
`prompt_tokens`, `completion_tokens`, `total_tokens`, `usage_rounds`, and when
the provider sent them: `cached_tokens`, `cache_write_tokens`,
`reasoning_tokens`, `prompt_audio_tokens`, `completion_audio_tokens`,
`accepted_prediction_tokens`, `rejected_prediction_tokens`, `service_tier`.

Do not scrape these with a hook inside the process. `docker logs` is pull.
Chars/4 estimates stay on `*_est_tokens`. Native `prompt_tokens` /
`completion_tokens` / `total_tokens` are the provider counts when present.

---

## Not this contract

- No listen port, `/metrics`, or scrape endpoint on the crane
- No instance slug / yard name inside the harness
- No dashboard write path on Completer rounds or tool fan-out
