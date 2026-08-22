# Observability (no dashboard, still measurable)

The harness ships **no metrics endpoint, no dashboard, no `/metrics` port** —
on purpose (no open ports, ever). That does not mean a long-horizon agent is
a black box. Everything people ask about — memory, GPU, timing, token spend
— is already observable from three places:

| Signal | Source | Where to look |
| --- | --- | --- |
| Per-turn trajectory + token estimates | gantry's JSON `slog` on stderr | `journalctl -u gantry` / `docker logs` |
| Slow / serial / recovery-heavy turns, memory, prompt size | slash commands (harness-side) | `/perf` · `/memstats` · `/toolstats` · `/tokens` in chat |
| Process CPU / RAM (gantry + MCP children) | the supervisor's cgroup accounting | `systemctl status` / `docker stats` |
| Model RAM / VRAM, residency, offload split | Ollama's own CLI + GPU tools | `ollama ps`, `nvidia-smi`, … |
| Memory rows, session size, disk | the SQLite file itself | `sqlite3 data/gantry.db` · or `/memstats` |

This page is the command reference. Interpretation and tuning levers live in
[deploy-native.md § Latency](deploy-native.md#latency-measure-before-tuning).

---

## The memory trap: `top` won't show your model

The number one confusion on a native box: Ollama loads model weights and the
KV cache into **GPU VRAM** through its runner process, so `top` / `htop` show
only a small host RSS for `ollama` — the multi-GB model is invisible there.
Ask Ollama and the GPU directly:

```bash
ollama ps
# NAME               SIZE     PROCESSOR    UNTIL
# qwen3.6:35b-a3b    23 GB    100% GPU     forever   ← keep-alive -1
```

Read it as:

- **SIZE** — weights + KV cache actually allocated for the loaded context
  (`OLLAMA_CONTEXT_LENGTH` inflates this, not the file on disk).
- **PROCESSOR** — the number that predicts speed. `100% GPU` is what you
  want; a `48%/52% CPU/GPU` split means layers spilled to system RAM and
  prefill/decode fall off a cliff.
- **UNTIL** — keep-alive. `forever` under `OLLAMA_KEEP_ALIVE=-1`; anything
  else means the next turn after idle pays a full model reload.

Per-vendor VRAM view (pick yours):

```bash
watch -n2 nvidia-smi          # NVIDIA: VRAM used, per-process
rocm-smi --showmeminfo vram   # AMD dGPU
amdgpu_top                    # AMD APU/iGPU (GTT = model in shared RAM)
intel_gpu_top                 # Intel
```

On APU / iGPU mini-PCs (unified memory) the "VRAM" is carved out of system
RAM as GTT, so `free -h` drops when a model loads even though no process
shows the usage — `amdgpu_top`'s GTT line is the honest number there.

The offload decision is also logged once at load time:

```bash
journalctl -u ollama --since -1h | grep -E 'offload|memory'
```

---

## gantry's own footprint (and the MCP children)

The harness is a single static Go binary; MCP tools are its child processes.
Under systemd they all live in **one cgroup**, so the unit's memory number
already includes every MCP server — no need to hunt PIDs:

```bash
systemctl status gantry       # Memory: line = gantry + all MCP children
systemd-cgtop                 # live per-unit CPU/mem, top-style
```

To break it down per process anyway:

```bash
ps -o pid,rss,etime,cmd --ppid "$(pgrep -x gantry)"   # each MCP child
ps -o pid,rss,etime,cmd -p "$(pgrep -x gantry)"       # the harness itself
```

Liveness is an exit code, not a port:

```bash
gantry status; echo $?        # 0 = heartbeat row is fresh
```

---

## Per-turn timing and token spend (the logs)

Every turn logs its own cost as JSON on stderr. The three lines that matter:

```bash
journalctl -u gantry -f | grep -E 'model call|tool done|turn perf'
```

| Log line | Key fields | Read it as |
| --- | --- | --- |
| `model call` | `first_token_ms`, `dur_ms`, `prompt_est_tokens`, `volatile_est_tokens`, `schema_est_tokens`, `tool_calls`, `finish_reason` | `first_token_ms` ≈ prefill; rest of `dur_ms` is decode; `*_est_tokens` are chars/4 estimates |
| `tool done` | `name`, `dur_ms`, `result_chars` | slow MCP vs slow model |
| `turn perf` | `source`, `user_id`, `session_id`, `iterations`, `tool_calls`, `max_batch`, `recoveries`, `prompt_est_tokens`, `gen_est_tokens`, `tools_per_inv`, `model_ms`, `tool_ms`, `total_ms`, `outcome` | Trajectory: work per Completer round; `user_id` is the channel identity gantree ranks spend by |

Because the logs are structured JSON, `jq` turns them into ad-hoc metrics —
`-o cat` strips journald's prefix so lines parse cleanly:

```bash
# Slowest turns in the last day: wall / model / tool + trajectory
journalctl -u gantry --since -1d -o cat \
  | jq -r 'select(.msg=="turn perf")
           | [.total_ms,.model_ms,.tool_ms,.iterations,.tool_calls,.max_batch,.recoveries,.prompt_est_tokens,.gen_est_tokens,.outcome] | @tsv' \
  | sort -rn | head

# Which tool is the bottleneck, by cumulative time
journalctl -u gantry --since -1d -o cat \
  | jq -r 'select(.msg=="tool done") | [.name,.dur_ms] | @tsv' \
  | awk '{sum[$1]+=$2; n[$1]++} END {for (t in sum) printf "%-40s %6d ms avg  %4d calls\n", t, sum[t]/n[t], n[t]}'

# Estimated prompt tokens per model call (spend proxy for cloud LLMs)
journalctl -u gantry --since -1d -o cat \
  | jq -r 'select(.msg=="model call") | .prompt_est_tokens' \
  | awk '{s+=$1; n++} END {print s" est tokens over "n" calls"}'
```

Token counts are chars/4 **estimates** everywhere (logs, `/status`) — good
for trends and comparisons, not billing math. For exact cloud spend, the
provider's usage console is the source of truth; these logs tell you *which
turn* caused it.

Boot cost is logged too — schema weight per MCP server:

```bash
journalctl -u gantry -b | grep -E 'tools_listed|tools_published|est_tokens'
```

In-chat, `/status` shows session bounds and estimates without touching the
host; `/tokens` breaks the standing prompt into persona / summary / history /
hydration / schemas; `/perf` / `/memstats` / `/toolstats` answer the usual
follow-ups (did that objective fan out or recover-loop, is memory
consolidating, which MCP is chronic) without SSH. `TELEGRAM_ERROR_REPORTING=error` tees error-level logs
into the chat as expandable blocks when you can't reach a terminal.

---

## Docker deployments

Same signals, container-shaped. The image is Distroless — **no shell inside**
— so all inspection happens from the host:

```bash
docker stats gantry           # live CPU / mem vs the compose mem_limit (1g)
docker logs -f gantry 2>&1 | grep -E 'model call|tool done|turn perf'

# jq needs the raw JSON — drop compose's service-name prefix:
docker compose logs --no-log-prefix --since 1h gantry \
  | jq -r 'select(.msg=="turn perf") | [.total_ms,.model_ms,.tool_ms] | @tsv'
```

`docker stats` counts the whole container cgroup, so MCP children are
included — same guarantee as the systemd unit. Health is the same exit-code
story (`docker inspect --format '{{.State.Health.Status}}' gantry`, backed by
`["CMD","gantry","status"]`).

In the usual Docker shape the model is a **cloud endpoint**, so there is no
GPU to watch — consumption is token spend, and the `model call` /
`prompt_est_tokens` recipes above are the tool. If you point a container at
host Ollama, the [memory trap](#the-memory-trap-top-wont-show-your-model)
section applies unchanged on the host.

---

## Disk: the SQLite file

One file holds sessions, memory, cron, and the heartbeat. Watch it like any
file, query it like any database ([memory.md](memory.md) goes deeper):

```bash
ls -lh data/gantry.db*        # includes -wal / -shm while running (WAL mode)

sqlite3 data/gantry.db "
  SELECT kind, COUNT(*) FROM memory GROUP BY kind;
  SELECT COUNT(*) FROM session;
  PRAGMA page_count; PRAGMA page_size;"
```

If the host has no `sqlite3`, copy `data/gantry.db` off the box and query it
locally.

---

## Cheat sheet

| Question you'll get asked | One command |
| --- | --- |
| "How much RAM is the bot using?" | `systemctl status gantry` (children included) / `docker stats` |
| "Where did my system RAM go?" | `ollama ps` + `amdgpu_top` (GTT) — not `top` |
| "Is the model still loaded?" | `ollama ps` → UNTIL column |
| "Why was that answer slow?" | `journalctl -u gantry -o cat \| jq 'select(.msg=="turn perf")'` |
| "Did it fan out or recover-loop?" | `/perf` → `iters` / `tools` / `batch` / `rec` |
| "Is it the model or a tool?" | `model_ms` vs `tool_ms` in the same line |
| "What did the trajectory cost in tokens?" | `turn perf` → `prompt_est_tokens` + `gen_est_tokens` (estimates) |
| "Is the bot alive?" | `gantry status; echo $?` |

If you genuinely need dashboards, don't add a port to gantry — ship the
journal (`promtail`, `vector`, `journald` → Loki) and graph the same JSON
fields there. The harness contract stays: logs out, nothing in.
