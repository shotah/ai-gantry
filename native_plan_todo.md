# native_plan — TODO (local Qwen/Ollama quality pass)

Follow-ups for the native deployment (SAM: Ubuntu + Ollama + Qwen3.5-35B-A3B).
Shipped so far: prompt-cache-friendly message order, keep-alive override, tool
trim (79→62), ISO dates in temporal anchor, unknown-tool suggestions,
underscore→hyphen prefix alias on MCP `Call`, think-stall nudge, thinking
accumulation in Telegram. Operator write-up: [docs/mcp.md](docs/mcp.md).
This file is what's next.

---

## P0 — Why does he get cut off mid tool call?

**Evidence (journal, Jul 27):**

```text
WARN model hit max_tokens (reply may be truncated) finish_reason=length chars=31579 iteration=1
WARN tool call failed  get_events: googleapi 404 (model passed a TIME RANGE as an event id)
```

31,579 chars ≈ the whole 8192-token completion budget in one iteration —
thinking + verbose prose eat the cap, and when the cutoff lands inside
tool-call JSON the call is truncated or dropped entirely. The 404 shows a
second failure mode: valid tool, wrong argument shape.

### Checklist

- [ ] **Instrument first:** when `finish_reason=length`, log whether tool-call
      arguments were mid-stream (truncated JSON) vs. plain text overflow
- [ ] **Detect + retry:** truncated/unparseable tool-call args → feed a tool
      error back ("arguments truncated — retry with shorter reasoning and
      minimal JSON"), same pattern as the unknown-tool suggestion
- [x] **Budget math:** `OLLAMA_CONTEXT_LENGTH=49152` now ships in
      `deploy/ollama-gantry.conf` (same drop-in as keep-alive; install.sh
      reinstalls on change) so `LLM_MAX_TOKENS` can stay generous without
      evicting persona
- [ ] **Schema misuse (404s):** argument validation errors already round-trip
      to the model — verify the message names the offending parameter; if the
      MCP's error is opaque, wrap it with the tool's schema summary

## P0 — Keep the agent finishing multi-step tasks

One think-stall nudge shipped. After tools, CoT-only answers are **promoted**
to the user reply (Jul 28 Garmin sleep: data OK, Qwen wrote the summary only
in thinking → ERROR). Remaining gaps:

- [x] Promote thinking → reply when `sawTools` and Content empty (no second nudge)
- [x] Nudge when prose promises a tool (“let me pull…”, `server__tool`) with no
      `tool_calls` (once, before tools)
- [x] Invented tool names now get closest-real-name hints, not just a prefix
      list (Jul 28: `mcp__get_hrv_and_body_battery` = fake prefix + `get_hrv`
      merged with `get_body_battery`; the bare prefix list told him nothing and
      he answered in prose instead of retrying)
- [x] Invented/missing prefix on a *real* tool name is now repaired in place
      (`mcp__get_hrv` → `garmin__hrv_get`), saving a ~9s round-trip. Not repaired
      when the prefix is a real server, or when two servers share the name
- [ ] Nudge on `finish_reason=length` with no usable output (today: warn only)
- [ ] `TOOL_MAX_ITERATIONS` exhaustion currently returns an error → user sees
      "something went wrong"; better: final forced completion with "answer with
      what you have, no more tools"
- [x] **Printed tool calls** (Jul 28, seen in the wild): after the prose nudge,
      Qwen answered with `{"name":"garmin__get_daily_activity","parameters":{…}}`
      as *content* and gantry sent that JSON to Telegram as SAM's reply. Now
      parsed back into a real call and executed (bare / fenced / `<tool_call>`
      tags / embedded in prose; `arguments`|`parameters`|`args`|`input`). Requires
      a tool-shaped name so a JSON answer is never hijacked
- [x] **Grammar-constrained repair retry** (shipped). After an unresolvable tool
      name, the next call sends `response_format` with an `enum` of the real
      candidates, so Ollama's GBNF masks any other name. Probed on 0.32.4:
      `tools` + `response_format` still suppresses `tool_calls` (call arrives in
      `content`), and dropping `tools` costs argument correctness — so keep both.
      One-shot and non-streaming, since a grammar forces every reply to be JSON
- [ ] Consider Ollama structured outputs / `format` for tool-call turns if
      truncated-JSON retries prove common

## P1 — Model strategy (routing + response time)

Decode is fine (~23 tok/s); the pain is long thinking before action.

Shipped (config + visibility, do the eval on top of these):

- [x] Per-turn perf logging: `model call` (`first_token_ms`, `dur_ms`,
      `prompt_est_tokens`, `tool_schemas`), `tool done` (`dur_ms`,
      `result_chars`), `turn perf` (`model_ms` / `tool_ms` / `total_ms`)
- [x] `LLM_REASONING_EFFORT=none` as the **native default** (thinking decoded
      at full price before any tool fires)
- [x] `TOOL_RESULT_MAX_CHARS=6000` native default (results re-prefill on every
      loop iteration, so the cap is a multiplier)
- [x] `OLLAMA_CONTEXT_LENGTH` drop-in (see P0 budget math)
- [x] Tool-call trace in the Telegram bubble — perceived latency, not real
- [x] **Prompt cache was never hitting:** `mcp.Host.Tools()` iterated a map, so
      the 13.5k-token schema block (63% of the prompt, leading the system
      message) was reshuffled every turn. Sorted by name → measured on SAM:
      warm turn 74.5s → 15.6s, `first_token_ms` 68.8s → 8.6s. The tell was
      iteration 2 of a turn prefilling in 1.8s (same order) while iteration 1
      of the next turn took 68s
- [x] `volatile_est_tokens` / `hydration_est_tokens` logging — `first_token_ms`
      tracks the re-evaluated tail, not the total prompt

Measured on SAM once the cache was hitting (18.4s turn, ~25.8k context):

| Phase | Cost | Rate |
| --- | --- | --- |
| Prefill 2,236 tokens (cache reused 23,561) | 8.5s | 264 tok/s |
| Decode 229 tokens | 10.0s | 23 tok/s |

Decode is now the bigger half. Two notes that killed earlier guesses:

- **Hydration is not the problem** — `hydration_est_tokens=48`, whole volatile
  tail 139 tokens. Lowering the 30-entry cap would save nothing.
- 139 changed tokens still cost 2,236 tokens of prefill because Ollama rewinds
  to the nearest **context checkpoint** (~2,044-token spacing), not to the
  divergence point. That granularity is Ollama-internal.

Remaining options, cheapest first:

- [x] **Reply length:** persona `SOUL.md` "Length (hard rule)" — 2–4 sentence
      default, no preamble/process-recap/closing offers. Persona lives in the
      cached prefix, so this trades free prompt tokens for paid output tokens
- [ ] **Lossless schema slimming:** strip `title` / `$schema` / `examples` from
      MCP-supplied JSON Schema (no capability loss) to cut cache-miss cost
- [ ] **Per-server schema accounting** at boot so trims are data-driven
- [x] `COALESCE_SETTLE_MS` used to apply to *every* message (2s on every reply).
      Now a lone bubble runs immediately; the settle only arms once a follow-up
      interrupts a running turn — which is what the feature was always for
- [x] **Perceived latency:** `SPINUP_NOTICE_MS` opens the bubble during silent
      prefill, then the reply replaces the line (transient status, not a trace).
      Immediate on the first turn after start (known-cold), threshold
      otherwise. `/api/ps` is useless as a cold signal under
      `OLLAMA_KEEP_ALIVE=-1` (`expires_at` = year 2318) and the KV prefix cache
      has no API, so observed silence is the only honest trigger

| Option | Cost | Notes |
| --- | --- | --- |
| Prompt-side: "act first, think less" persona line for tool turns | free | try before anything structural |
| `qwen3.6:35b-a3b` / `qwen3.6:27b` / smaller instruct for tool selection | pull + eval | same family, faster decode |
| Two-model router: small model picks tool + args, 35B composes reply | code | biggest win if selection is the bottleneck; adds real complexity |
| Function-calling-tuned model as the single brain | pull + eval | risks losing SAM's voice; needs persona regression check |

- [ ] Define a 10-prompt eval script (calendar day query, strava summary,
      math, multi-tool chain) with expected tool calls; time-to-first-token +
      correctness per model — **decide with data, not vibes**
- [ ] Only then decide router vs. single-model

## P1 — Math MCP (import, don't write)

Qwen does arithmetic in CoT and gets it wrong; a calculator tool ends that.

- [x] Survey existing MCP calculator/math servers; prefer a **static Go
      binary** (fits the native/no-Docker deploy like the other tools)
- [x] Must be stdio MCP, few tools (1–3: evaluate, maybe unit-convert) — do
      not add another 15-tool surface
- [x] Wire into `mcp.toml` + `TOOLS.md` note: "use math tool for any
      arithmetic beyond trivial"
- [x] Existing Go calculators expose 13+ tools (too fat for Qwen) → tiny
      [shotah/mcp-go-math](https://github.com/shotah/mcp-go-math) `v0.0.2`
      (`evaluate` + `convert`); Dockerfile pin + `download_url` in mcp.toml
      (source-agnostic; `gantry tools-plan` → native fetch)
- [ ] Deploy to SAM (`make remote-native-deploy-dev`) and smoke
      `math__expression_evaluate`

## P1 — Errors → Telegram (red collapsed box)

Viable: yes. Telegram has no red styling, but `🔴 <b>gantry ERROR</b>` +
`<blockquote expandable>` gives exactly "collapsed box I click to open" — same
mechanism as the thinking display.

**Design (slog handler → same SAM DM):**

- `slog.Handler` wrapper that tees `ERROR` (and optionally `WARN`) records into
  the existing Telegram chat as HTML: emoji header + expandable blockquote
- **Loop guard (critical):** never forward errors emitted by the Telegram
  sender itself; drop instead of retry when Telegram is down (see Jul 27
  03:00 `getUpdates` timeouts — forwarding those would recurse)
- Dedupe/rate-limit: same `msg` key ≤ 1 per N minutes, counter in the box
- Config: `TELEGRAM_ERROR_REPORTING=off|error|warn` (default off) — no new chat / secret

### Checklist

- [x] `internal/logfwd`: slog tee handler + 5m dedupe window
- [x] Telegram formatter: header + expandable blockquote, clip to 3500 runes
- [x] Loop guard + drop-on-failure semantics + tests
- [x] Env plumbing + docs (`local-agent/docs/telegram.md`)
- [x] Native default `TELEGRAM_ERROR_REPORTING=error` via `remote-native-env`

## Non-goals

- Multi-channel error fan-out (Telegram only; it's where the operator lives)
- Replacing Ollama or the OpenAI-compat provider layer
- Embeddings/vector memory (FTS5 still fine at this scale)
- NPU/XDNA acceleration work
