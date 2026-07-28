# native_plan — TODO (local Qwen/Ollama quality pass)

Follow-ups for the native deployment (Tim: Ubuntu + Ollama + Qwen3.5-35B-A3B).
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
- [ ] **Budget math:** consider `OLLAMA_CONTEXT_LENGTH=49152` override (same
      systemd drop-in as keep-alive; 87GB unified RAM has headroom) so
      `LLM_MAX_TOKENS` can stay generous without evicting persona
- [ ] **Schema misuse (404s):** argument validation errors already round-trip
      to the model — verify the message names the offending parameter; if the
      MCP's error is opaque, wrap it with the tool's schema summary

## P0 — Keep the agent finishing multi-step tasks

One think-stall nudge shipped. Remaining gaps:

- [ ] Nudge on `finish_reason=length` with no usable output (today: warn only)
- [ ] Nudge when the model emits prose *about* calling a tool but no tool call
      for N consecutive iterations (detect tool names in text?) — cheap heuristic,
      measure before building
- [ ] `TOOL_MAX_ITERATIONS` exhaustion currently returns an error → user sees
      "something went wrong"; better: final forced completion with "answer with
      what you have, no more tools"
- [ ] Consider Ollama structured outputs / `format` for tool-call turns if
      truncated-JSON retries prove common

## P1 — Model strategy (routing + response time)

Decode is fine (~23 tok/s); the pain is long thinking before action. Options,
cheapest first:

| Option | Cost | Notes |
| --- | --- | --- |
| Prompt-side: "act first, think less" persona line for tool turns | free | try before anything structural |
| `qwen3.5:35b-a3b` variants / smaller instruct for tool selection | pull + eval | same family, faster decode |
| Two-model router: small model picks tool + args, 35B composes reply | code | biggest win if selection is the bottleneck; adds real complexity |
| Function-calling-tuned model as the single brain | pull + eval | risks losing Tim's voice; needs persona regression check |

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
- [ ] Deploy to Tim (`make remote-native-deploy-dev`) and smoke
      `math__evaluate`

## P1 — Errors → Telegram (red collapsed box)

Viable: yes. Telegram has no red styling, but `🔴 <b>gantry ERROR</b>` +
`<blockquote expandable>` gives exactly "collapsed box I click to open" — same
mechanism as the thinking display.

**Design (slog handler → same Tim DM):**

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
