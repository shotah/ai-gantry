# Deploy: Native (Linux + systemd + local model)

Run the same **static `gantry` binary** on the host under systemd — typically
next to [Ollama](https://ollama.com) and a local chat model (we use
**Qwen** in production). No Docker on the agent box; MCP tools are plain
binaries on `PATH`.

Kernel contract: [root readme](../readme.md). Tool naming: [mcp.md](mcp.md).
Consumer template (unit + `install.sh`): [examples/native/](../examples/native/).
Appliance Make targets / host layout: [local-agent/deploy/README.md](../local-agent/deploy/README.md).

```mermaid
flowchart LR
  TG[Telegram] <-->|outbound long-poll| G
  subgraph Host["Linux host"]
    G[gantry · systemd]
    O[Ollama · Qwen]
    M[MCP binaries on PATH]
    G -->|OpenAI-compat localhost| O
    G -->|stdio| M
  end
```

---

## Why native + local models

| Goal | Native path |
| --- | --- |
| Own the weights | Ollama (or any OpenAI-compat) on the same box |
| Skip container tax | One process, journald logs, host networking for Cast/mDNS |
| Same agent contract | Identical env + persona + `mcp.toml` as Docker |

Cloud LLMs still work (`LLM_*` → Gemini/Grok). Native shines when the brain
is local and you care about RAM, keep-alive, and tool-call quality.

---

## Featured stack (local-agent appliance)

Production shape we run today:

| Piece | Choice |
| --- | --- |
| Chat | Ollama · `qwen3.6:35b-a3b` (or current pin) |
| Channel | Telegram allowlist |
| Supervisor | systemd unit under `/opt/gantry` |
| Tools | Garmin, Workspace, search, Cast, … via `mcp.toml` |

Workstation → remote host:

```bash
cd local-agent
# .env: DEPLOY_HOST, DEPLOY_USER, DEPLOY_PATH=/opt/gantry, Telegram, …

make remote-native-env      # gantry.env with Ollama LLM_* defaults
make remote-native-check
make remote-native-deploy   # release binary → install → systemctl start
# day-to-day from a dirty tree:
make remote-native-deploy-dev
```

Host layout, secrets under `data/.config/`, cutover from Docker SAM:
**[local-agent/deploy/README.md](../local-agent/deploy/README.md)**.

Minimal `LLM_*` for Ollama on the same machine:

```env
LLM_BASE_URL=http://127.0.0.1:11434/v1
LLM_API_KEY=ollama
LLM_MODEL=qwen3.6:35b-a3b
```

---

## How gantry tackles local-model rough edges

Small / thinking models (Qwen) are great offline brains but messy with tools.
The kernel hardens the loop so SAM finishes multi-step turns:

| Failure mode | What gantry does |
| --- | --- |
| Fat MCP catalogs drown Flash/Qwen | Manifest `tools` / `exclude` + MCP `--tool-tier` (e.g. Garmin `core`) |
| Hyphenated prefix mangled to underscores (`google_search__…`) | Host aliases prefix `_`→`-` when the catalog matches ([mcp.md](mcp.md)) |
| Invented tool names (`…__web_search`) | Model-facing catalog suggestions on unknown tools |
| Answer stuck in CoT after a successful tool | Promote thinking → user reply (no ERROR stall) |
| Prompt cache thrash | Stable message prefix (persona / summary / history); volatile blocks last |
| Schema token blowups | `TOOL_SCHEMA_MAX_TOKENS` + boot `est_tokens` logging |

Persona still spells exact names (`TOOLS.md`). Runtime fixes catch the rest.

---

## Latency: measure before tuning

On a local model, "slow" is almost never decode speed. Decode is steady; the
wait is **prefill** (persona + tool schemas + history re-evaluated) and
**thinking tokens spent before the first tool call**. Both are visible in the
journal — `gantry` logs every turn (full command reference for memory / GPU /
timing: [observability.md](observability.md)):

```bash
journalctl -u gantry -f | grep -E 'model call|tool done|turn perf'
```

| Log line | Fields | Read it as |
| --- | --- | --- |
| `model call` | `first_token_ms`, `dur_ms`, `volatile_est_tokens`, `prompt_est_tokens`, `schema_est_tokens` | `first_token_ms` ≈ prefill; the rest of `dur_ms` is decode |
| `tool done` | `dur_ms`, `result_chars` | Slow MCP vs slow model; `result_chars` lands in the volatile tail |
| `turn perf` | `iterations`, `model_ms`, `tool_ms`, `total_ms`, `hydration_est_tokens` | Which half of the turn to attack |

**`volatile_est_tokens` is the number that predicts `first_token_ms`, not
`prompt_est_tokens`.** The prefix (persona + summary + history) is byte-stable
across turns and gets cached; only the tail — hydration, clock, user message,
and tool results from earlier iterations — is re-evaluated. A warm turn with a
big total prompt is fine; a warm turn with a big *volatile* tail is not.

Cross-check the model side against Ollama's own numbers (`prompt eval count`
and `eval count` with per-token timings):

```bash
journalctl -u ollama -f
```

### Levers, cheapest first

| Lever | Where | Effect |
| --- | --- | --- |
| Stable prompt prefix | code — persona/summary/history first, volatile last; `Host.Tools()` sorted | Biggest single win measured on Qwen/890M: a reshuffled tool block broke the prompt cache and cost ~68s of re-prefill per turn instead of ~2s |
| `OLLAMA_CONTEXT_LENGTH` | [`deploy/ollama-gantry.conf`](../local-agent/deploy/ollama-gantry.conf) | Ollama's default `num_ctx` is small; overflowing it forces context shifting + re-prefill every turn |
| `OLLAMA_KEEP_ALIVE=-1` | same file | Model stays resident; confirm with `ollama ps` (want `100% GPU`) |
| `LLM_REASONING_EFFORT=none` | `gantry.env` | Native default. Thinking tokens decode at full price *before* any tool fires |
| `TOOL_RESULT_MAX_CHARS` | `gantry.env` | Native default `6000`. Results are re-sent each loop iteration, so this multiplies prefill |
| Shorter replies | persona (`SOUL.md` → "Length") | Decode is a hard ~23 tok/s: a 230-token reply *is* 10s. Halving reply length halves that. Persona text is in the cached prefix, so it costs nothing per turn |
| Fewer tools | `mcp.toml` `tools` / `exclude`, MCP `--tool-tier` | Schemas are cached once the prefix is stable, but they inflate total context — and prefill rate falls with length (~1000 tok/s at 16k vs ~264 tok/s at 25k) |
| `COALESCE_SETTLE_MS` | `gantry.env` | Deliberate quiet window before a turn starts — counts against time-to-first-response |
| `SPINUP_NOTICE_MS` | `gantry.env` | Doesn't make a turn faster — opens the bubble during silent prefill so it stops *feeling* frozen |
| `OLLAMA_FLASH_ATTENTION` / `OLLAMA_KV_CACHE_TYPE=q8_0` | `ollama-gantry.conf` (commented) | Faster prefill, much smaller KV cache; measure quality before keeping |
| Smaller / router model | `LLM_MODEL`, or a second endpoint | Real work — only worth it once the logs say model time dominates |

`install.sh` reinstalls `ollama-gantry.conf` whenever it changes and restarts
Ollama, so the first turn after a tuning change is cold. Unchanged means no
restart, so ordinary redeploys keep the model resident.

### Perceived latency

Tool chains stream a trace into the Telegram bubble (`→ garmin__activities_list`
then `✓ 1.2s · 4.1k chars`), so a long turn shows motion instead of looking
frozen. With `LLM_REASONING_EFFORT=none` that trace is the whole expandable
block. Needs `STREAM_REPLIES=true`.

Prefill itself is silent, so `SPINUP_NOTICE_MS` (default `4000`) opens the
bubble with a "hang on" line before the first token:

- **First turn after gantry starts** posts a random cold-start line immediately —
  that turn is known-cold (model load and/or an empty prompt cache) and measures
  ~76s against ~15s in steady state.
- **Later turns** post a random "still working" line only after the threshold,
  which covers a prompt-cache miss. Nothing in an OpenAI-compatible API reveals one:
  `ollama ps` reports the model resident (`expires_at` in the year 2318 under
  `OLLAMA_KEEP_ALIVE=-1`) whether the turn takes 15s or 76s, and the KV prefix
  cache has no API at all. Observed silence is the only honest signal.

Unlike a tool trace the notice is transient — the first token clears it and the
reply takes the bubble, so it never lingers in the finished message.

---

## REPL / hack loop (no systemd)

From the repo root, against any OpenAI-compat endpoint (including Ollama):

```bash
make init
# deploy/mcp.toml + .env — set LLM_* to Ollama or Gemini
make run    # CHANNEL=stdio
```

`/status` · `/tools` · ask for a dated tool call. See [mcp.md](mcp.md).

---

## When to prefer native

| Prefer native when… | Prefer [Docker](deploy-docker.md) when… |
| --- | --- |
| Ollama/Qwen (or other local) on a mini-PC | Hub image + compose is enough |
| Cast / LAN tools want host network simply | Distroless grant story matters most |
| You already live in systemd + journalctl | Workstation has Docker; server gets `remote-deploy` |

Same binary, same mounts-or-paths contract — only the supervisor changes.
