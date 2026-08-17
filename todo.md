# ai-gantry — todo

**What to investigate next.** Features, token work, and loop improvements.
The agent already runs. Deleted ideas stay deleted — they were non-starters,
not a backlog. This is not a lockdown / hardening / “safe the agent” list.

Shipped work is not listed here. See [docs/milestones.md](docs/milestones.md),
[docs/choices.md](docs/choices.md), [docs/watch.md](docs/watch.md).

Status: **next** · **later** · **maybe** · **prototype**
Size: **S** ≈ an afternoon · **M** ≈ a weekend

---

## Fit gates

1. **Outbound by default.** Prefer poll / long-poll / outbound WebSocket
   (what Telegram/Slack already are). An inbound port is discussable if we
   can say why poll is not enough **and** how it stays locked down (not
   `0.0.0.0` + hope).
2. **MCP or nothing.** Kernel work only for things a tool server cannot do
   (channel, loop, memory, cron, and a watch wake).
3. **Native compiled binary. No JIT.** Go / C / C++ / Rust. Never Python,
   Node, `npx`, `uvx`, Bun, JVM. Things we author stay **Go**
   (`CGO_ENABLED=0`).
4. **Static child** `gantry tools-fetch` can pin. Distroless has no runtime.
5. **Import over write.**
6. **Tiny catalog.** Every tool schema is re-billed every turn.
7. **Still 1:1.** One persona, one model, one channel. Not multi-agent. Not
   provider orchestration.

---

## Next

### 1. Model + tool profiles — **next** · M

Not multi-agent. Not provider fallback. One process, one Completer, a
**paired** tool surface:

```text
cheap / local model  →  small tool surface
smart / cloud model  →  broad tool surface
```

Flash and Qwen already degrade when fed ~150 schemas
([choices.md](docs/choices.md#tool-surface-budget)). Curation
(`mcp.toml` `tools` / `exclude` / `--tool-tier core`) is the operator
doing this by hand. A profile makes the pairing explicit so swapping
`LLM_MODEL` also swaps the published set — no second agent, no mid-turn
expand dance.

**Keep always-on:** memory / self_note / calendar-ish core. Defer
flights / rentals / youtube until a week you actually use them.

Do this **before** dynamic grouping. Profiles are static and
cache-friendly: the tools array stays byte-stable for the life of the
process. Expanding a server mid-chat busts the prefix cache and costs
another Completer call.

---

## Later

### Dynamic tool grouping — **later** · M

A lightweight way to give the model the **right tools for this turn**,
rather than all of them. Live `/tokens` has shown `schemas` in the same
order as history (~17k).

**Do first (no new loop):** profiles + `mcp.toml` `tools` / `exclude` /
`--tool-tier`. That is progressive loading with the operator as the
search tool.

**Cache (“send once”) is already the local/cloud story** if the tool
list never changes. OpenAI/Gemini cache the tools array as part of the
prefix; Ollama KV-cache does the same. Gantry already sorts
`Host.Tools()` so a reshuffle does not cost a full re-prefill.
`/tokens` still prints the full send size, not the discounted bill.
You cannot omit tools on later turns and still call them — the model
only sees this request.

**Expand-on-demand is a real 2026 pattern** (Anthropic `defer_loading`
+ tool search). On our OpenAI-compat loop we would own it: builtin
`mcp_open(server)` swaps the published set, extra turn, then the real
call. Small local models already misspell names — a two-step “pick
server, then tool” is another miss. Extra Completer call = persona +
history billed again, and the tools-array change **busts** the prefix
cache.

Build the expand dance only if a curated + profiled catalog is still
>~8k on `/tokens` *and* we are willing to spend a turn + bust cache to
open a server.

| Item | Why | Size |
| --- | --- | --- |
| **Tone regression probe** | Fixture transcript with a planted running joke → fold → fresh context + summary → ask for the callback. One LLM grade, not CI-blocking. Run when a summary/distill prompt changes. | S |
| **Voice notes in** | Telegram voice → OpenAI-compat `/v1/audio/transcriptions` (`whisper.cpp` locally). Same tagged-text path as photos. | M |
| **Tiered history (LLM one-liners)** | Go word-list strip already shipped (last 5 verbatim, quotes kept). Middle tier — `user asked X / agent did Y, joked Z` — only if `/tokens` still says history dominates after 32k + the strip. | M |

---

## Prototype / maybe

### Tool-result caching — **skip** unless `/toolstats` shows repeat `{tool, args}`

A host TTL on identical `Host.Call` keys. Sounds cheap. The live agent
does not do this: he sees the result in the prompt and is **happy not
to call again**. The actual fight is getting a second pull, not
deduping a storm of the same search.

That item was an API-bill / runaway-automation holdover. Watch ticks
already have a cursor. In-turn old results already collapse. Revisit
only if `/toolstats` shows the same key firing over and over.

### Stateful objectives — **skip** unless the stack below actually fails

Default answer: **already covered.**

| Layer | Holds |
| --- | --- |
| `SELF.md` aims | Standing “poke before Thursday flights” |
| `Facts:` / memory | This week’s job, names, open loops |
| Cron + watch | The actual wakes |
| Google Tasks (or calendar) | A real long-running to-do with a due date |

A kernel “objective row” would only be a folder that stamps `[objective 7]`
onto those wakes so they share notes. If you are so deep in parallel jobs
that memory and Tasks cannot hold it, the fix is better task tracking —
not a second job database inside gantry.

Revisit only if a watch fire and a Thursday cron regularly *do not*
recognize they are the same job, and Tasks would not have fixed that.

| Item | Why | Size |
| --- | --- | --- |
| **Append-only summary epochs** | Each fold rewrites the whole summary → busts the cached prefix. Fold *adds* a paragraph; rare compaction merges old epochs. Cuts prefill latency, not billed size — local models. Measure `first_token_ms` first; skip if folds are rare. | M |
| **Hydration dedup vs summary/SELF** | `[memory]` re-states things the summary or SELF already carry. Skip rows whose content substring-matches. Ceiling is 30 rows — check `hydration_est_tokens` first. | S |
| **Outbound HTTP MCP** | Host only speaks stdio today. Unlocks Home Assistant (and other HTTP MCP) without opening a port. No proxy sidecar. | M — only if we actually want HA |
| **Webhook inbound** | Poll is minutes. A listen port is justified only if the source has a webhook, no decent poll/RSS, *and* waiting loses the moment. Bind Tailscale/localhost, HMAC on every POST, body is untrusted text, fail closed. Not a dashboard. | M — only for a source we cannot poll |

---

## Token costs (how to judge a cut)

Not all tokens cost the same. `/tokens` already prints the live table
(persona / summary / history / hydration / schemas). Use it before
justifying a cut.

| Bucket | What | Cut it by |
| --- | --- | --- |
| **Billed prompt tokens** | Cloud APIs re-bill the whole prompt every turn (cached prefix is discounted, not free) | Smaller persona / history / hydration / schemas |
| **Prefill latency** | Local models pay wall-clock for every *uncached* token | Byte-stable prefix; less churn, not just less text |
| **Context ceiling** | Small local models drown past ~8–32k | Bounded everything (mostly done) |

**Churn is a cost even when size isn't.** A 2k-char summary that rewrites
itself every fold invalidates the KV cache for everything after it.
Sometimes the win is *stop rewriting*, not *write less*.

Tone rule: any lossy idea has to answer “where does the tone go instead?”
The old summarizer was told to *“Drop chitchat”* — which is exactly where
the jokes live. Voice already lives in `Facts:` / `Voice:` in the session
summary, and in `SELF.md`. Do not store the funny only in the compressible
layer.

---

## Revert plan (history 32k + filler strip)

Not a todo. Written down so a bad week has a ladder, not a scramble. A
tagged release is the hard floor.

**When to even look:** a running joke or nickname that was in recent
chat fails a callback after a trim, `/tokens` `summary` has no `Voice:`
line, or the agent feels lobotomized after a long session (not after
`/new` — that is a different path). One miss is a probe candidate; a
pattern is a revert.

### Rung 1 — env only (minutes, no rebuild)

```bash
HISTORY_MAX_TOKENS=128000
```

Restart. New turns stop folding at 32k. Already-folded `Facts:` /
`Voice:` stay in `session.summary` (harmless).

To disable only the word-list strip (keep the 32k fold):

```bash
HISTORY_STRIP_FILLERS=false
```

Prompt-only — stored chat is already full wording.

### Rung 2 — leave the ledger, undo only the cut in git

Revert the default in `internal/config/config.go` (`envDefault:"32000"`
→ `"128000"`), the `session.Open` fallback, `.env.example` comments, and
the readme / config test. Keep `/tokens`, the summarizer prompt,
`Facts:` / `Voice:`, `/new` handoff, tool-arg collapse, and the persona
diet.

### Rung 3 — tagged release

Deploy the release cut before that work. Last resort. Memory rows and
`SELF.md` written after that release stay on disk — prune by hand.

**Do not revert** to “fix” a single bad `SELF.md` line or one parked
episode. Delete the line / `memory_forget` the row.

---

## Not doing

Deleted todos stay gone. Also:

| Idea | Why not |
| --- | --- |
| **Full github-mcp catalog** | He is not a dev assistant. A release / Actions **watch** is an event source, not 50 GitHub tools. |
| **filesystem MCP** | Docs/Drive already exist. A vault is a second store. |
| **weather / net-probe / nest / shipments / sky / mealie** | Search, Cast, Tasks, and calendar already cover the asks. New binary needs a daily question we cannot answer today. |
| **Paperless / Grafana / k8s** | Wrong persona. Fat catalogs. |
| **Python / Node / JIT MCP** | Anywhere. Write or import a native binary. |
| **Agent lockdown** (confirm-before-send, untrusted-forward wrappers, …) | He has been running fine. Do not invent a second ACL. |
| **Schema slimming** | The bytes you’d cut *are* the tool manual (descriptions, examples, titles). A “lossless” pass is just `$schema` / `$id` URIs. A pass that removes the manual damages call quality. Token lever that does not touch the manual: publish fewer tools (`tools` / `exclude` / `--tool-tier` / profiles). |
| **LLMLingua / a compression sidecar** | Python/torch (fit gate 3). The Go word-list strip is the in-process version. |
| **LLMLingua / perplexity-based token pruning** | Same sidecar reject, and it deletes “low-information” tokens — which is precisely what a joke looks like to a perplexity filter. |
| **Embedding-based semantic dedup / retrieval summaries** | Already rejected for memory ([choices](docs/choices.md)); same reasons — second model, opaque, unfixable when wrong. FTS5 + prompts is enough at one-user scale. |
| **Compressing SELF.md harder** | It's 4 KB, capped, and it *is* the tone. Tokens are cheap here at any price. |
| **A second “cheap summarizer model”** | 1:1, one model (fit gate 7). Two models means two voices summarizing one relationship. |
| **Watches as `cron_schedule` + a fetch prompt** | Quiet ticks must not call the Completer. Shipped the other way; do not rebuild it. |
| **Multi-agent / provider orchestration** | Fit gate 7. Profiles pair *this* model with *this* catalog. They do not route across brains. |

---

## When something ships

Docs in the same change: env in [readme §4.1](readme.md#41-environment-variables)
when a new var appears; MCP page + `mcp.toml` snippet when the catalog
changes. Then delete the row here.
