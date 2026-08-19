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

### 1. Prefix enable (brief / short) — **next** · M

The token / usability win. Not model+tool profiles — that is
operator fiddling (`mcp.toml` `tools` / `exclude` / `--tool-tier`
already does the static cut). Usage-shaped publish is the lever.

Not a router LLM. Not a new subset every bubble. Not a forever pin.
The agent gets a **stable index** of name prefixes and a builtin to
mark one **active**. Matching schemas join the published set on the
**next Completer call** (same `Handle`, next tool-loop iteration —
do not wait for the next user bubble, or the turn dies as “ok,
flights is on, what did you want?”).

A key is a **tool-name prefix**, not an MCP process. One rule for
Google, Garmin, and anyone else's monster catalog:

```text
flights              →  flights__*
google               →  google__*          (whole server; usually too fat)
google__calendar     →  google__calendar_*
google__gmail        →  google__gmail_*
garmin               →  garmin__*
garmin__sleep        →  garmin__sleep_*
```

`HasPrefix(tool.Name, key)` is the filter. Enabling `google` still
works; daily furniture should be `google__calendar`, not `google`.
A fat third-party MCP is the same: enable `monster__invoices`
without taking `monster__admin`. No extra binaries. The index lists
the server prefix and, when that server is fat, the next segment
already in the names. `mcp.toml` force-on / force-off still wins.

Store **`last_used` + hold** (`brief` | `short`). Idle is computed
(`now - last_used > window`). No `expires_at`. A successful call
refreshes the **longest matching** row (`google__calendar_list_events`
touches `google__calendar`, not a sibling `google__gmail`).

```text
mcp_enable prefixes=["google__calendar"]           # short (default)
mcp_enable prefixes=["flights"] hold=brief
mcp_enable prefixes=["google__calendar","garmin__sleep","strava"]
  → last_used = now, hold = brief|short  (same hold for the list)
  → next Completer call publishes all matching schemas
successful google__calendar_list_events
  → last_used = now on the longest matching row (hold unchanged)
quiet cron/watch tick (no Completer)
  → drop any row where now - last_used > that hold's window
```

One builtin, list argument — not a second `mcp_enable_list` schema.
A morning brief is still **one** extra Completer round (enable the
set, then the real calls), not one enable per prefix. Unknown keys
fail that item and the rest still enable; the tool result names
what landed. Cap the list (e.g. 8) so “enable the world” is one
blocked call, not a silent full catalog.

**Brief / short — better words than lease / pin.** Pin sounded
permanent. Both are “in use,” both fall off. The only difference
is the gap you will tolerate. No third hold: weekend or skip-a-day
silence is one `mcp_enable` when you come back, not leftover
schemas on every unrelated chat.

| | Brief | Short (default) |
| --- | --- | --- |
| Idle | 6h | 27h |
| Meaning | This morning / afternoon | Current job |
| Example | flights today only | flights this week |
| Who sets | agent `hold=brief` or `/brief` | agent default or `/short` |

6h = this morning or afternoon (flights after lunch, gone by evening).
27h = a day plus morning slack (7am → 9:30 next day still hot).

The agent sets both. Default is short. Brief is “only this
half-day.” Human: `/brief` `/short` `/off`.

Kernel builtins (memory / `self_note` / cron / watch / `mcp_enable`)
are actually always-on. `/tools` shows `brief` / `short` /
`available`.

**Small models / rollback:** `dynamic_tools = false` at the top of
`mcp.toml` publishes the full catalog every turn (today's
behavior). No `mcp_enable`, no idle drop. Default when the key is
omitted is **true**.

When dynamic tools are on, pin furniture without a model call:

```text
MCP_ENABLE_FORCE=google__calendar,garmin__sleep
```

`mcp.toml` `force = true` on a `[[server]]` adds that server's
prefix to the same always-on set. `exclude` still wins at the host.

**No schema-token hard cap / LRU eviction in v1.** Enable only
publishes a subset of the host catalog, so the published set
cannot exceed what we send today (everything on). A busy day
that enables flights + mail + maps is *today's* bill for 27h,
then it shrinks. Idle clocks are the pressure.

A cap that evicts `max(now - last_used)` would surprise a still-
live job (“flights was 3h ago, I enabled six other things, now
the schema is gone mid-search”). Revisit only if `/tokens` shows
the agent enabling the world and leaving it hot. Then: evict
unforced rows by greatest idle, never kernel / `mcp.toml`
force-on, stop when under the cap. `TOOL_SCHEMA_MAX_TOKENS`
stays the boot backstop for the *full* catalog, not this loop.

**Persist** by `session_id` in SQLite (same family as `/examples`
on/off). A process restart must not wipe a three-day search. 1:1
is usually one session; a second allowlisted DM does not fatten
this chat.

**Cache:** the tools array is byte-stable *between* enable/drop
events. Opening or dropping a prefix busts the prefix cache
**once**. That is cheap next to unused `flights__*` on every
message. A per-turn router busts it every message — do not
build that.

**Why this beats `mcp_open` as a one-shot expand:** the first
miss still costs an extra Completer iteration (enable, then
the real call). After that the schemas stay hot across the hold
instead of falling off at end of turn. Small models still have
to spell the key right — keep the index exact names, no fuzzy
“travel tools.”

**Go live: leave them off.** No short-seed of today's catalog.

An honest extra `mcp_enable` (list the prefixes, then the real
calls) is cheaper than a fat day-1 and a surprise shrink on day
2. Empty rows stay empty. The index is always visible so the
model can enable in the same turn. Token win starts on minute
one.

A newly connected MCP stays **off** until `mcp_enable` too.
Do not seed on process restart. `/new` must not wipe rows
(same Telegram chat / `session_id`) — once something is in
use, it stays across a session reset.

`mcp.toml` force-on is the only “already on” besides kernel
builtins (memory / `self_note` / cron / watch). Use it for
furniture you refuse to pay enable for on the first morning
(calendar / Garmin sleep). Everything else starts from 0.

---

## Later

| Item | Why | Size |
| --- | --- | --- |
| **Tone regression probe** | Fixture transcript with a planted running joke → fold → fresh context + summary → ask for the callback. One LLM grade, not CI-blocking. Run when a summary/distill prompt changes. | S |
| **Voice notes in** | Telegram voice → OpenAI-compat `/v1/audio/transcriptions` (`whisper.cpp` locally). Same tagged-text path as photos. | M |
| **Tiered history (LLM one-liners)** | Go word-list strip already shipped (last 40 verbatim, assistant never stripped, quotes kept). Middle tier — `user asked X / agent did Y, joked Z` — only if `/tokens` still says history dominates after 32k + the strip. | M |
| **Apple stack MCP** | Need: APPLE. Reminders / Mail / Calendar. | [apple_todo.md](apple_todo.md) |
| **Microsoft stack MCP** | Need: WORK. Outlook / Teams / To Do. | [ms_todo.md](ms_todo.md) |
| **AWS MCP** | Need: BILLING (then alarms). Not the 60-server catalog. | [aws_todo.md](aws_todo.md) |
| **GCP MCP** | Need: BILLING. Not `google-mcp`. | [gcp_todo.md](gcp_todo.md) |

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

### Webhook inbound — **maybe** · M (still want this)

External sources waking him. Same `Handle` as cron/watch, just a POST
instead of “we went and looked.” The like is real. The hold is fit
gate 1: a listen port.

Not the same item as **outbound HTTP MCP** (gantry dials Home Assistant
— no inbound port). This one is someone dialing *us*.

Ship only when all three are true:

1. The source has a webhook, no decent poll/RSS, **and** waiting until
   the next watch tick loses the moment.
2. Bind is Tailscale or localhost — not `0.0.0.0` + hope.
3. HMAC (or equivalent) on every POST; body is untrusted text; fail
   closed. Not a dashboard.

If (2)+(3) still feel wrong, the outbound-only shape is a tiny relay
you already trust (Telegram is this): Google POSTs at the relay, gantry
long-polls the relay. More moving parts. Same wake. No port on gantry.

| Item | Why | Size |
| --- | --- | --- |
| **Append-only summary epochs** | Each fold rewrites the whole summary → busts the cached prefix. Fold *adds* a paragraph; rare compaction merges old epochs. Cuts prefill latency, not billed size — local models. Measure `first_token_ms` first; skip if folds are rare. | M |
| **Hydration dedup vs summary/SELF** | `[memory]` re-states things the summary or SELF already carry. Skip rows whose content substring-matches. Ceiling is 30 rows — check `hydration_est_tokens` first. | S |
| **Outbound HTTP MCP** | Host only speaks stdio today. Unlocks Home Assistant (and other HTTP MCP) without opening a port. No proxy sidecar. | M — only if we actually want HA |

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

**Recovery that is not this ladder:** [voice_restore_todo.md](voice_restore_todo.md)
(spare assistant history, richer `Voice:`, distill merge, SOUL/SELF/RULES
stamps). Prefer that before rung 2.

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
| **Schema slimming** | The bytes you’d cut *are* the tool manual (descriptions, examples, titles). A “lossless” pass is just `$schema` / `$id` URIs. A pass that removes the manual damages call quality. Token lever that does not touch the manual: publish fewer tools (`mcp_enable` + `mcp.toml` `tools` / `exclude` / `--tool-tier`). |
| **Model + tool profiles** | Second catalog pairing when `LLM_MODEL` changes. Operator already has `mcp.toml`. Prefix enable is the usage-shaped cut — do not fiddle with a profile file. |
| **LLMLingua / a compression sidecar** | Python/torch (fit gate 3). The Go word-list strip is the in-process version. |
| **LLMLingua / perplexity-based token pruning** | Same sidecar reject, and it deletes “low-information” tokens — which is precisely what a joke looks like to a perplexity filter. |
| **Embedding-based semantic dedup / retrieval summaries** | Already rejected for memory ([choices](docs/choices.md)); same reasons — second model, opaque, unfixable when wrong. FTS5 + prompts is enough at one-user scale. |
| **Compressing SELF.md harder** | It's 4 KB, capped, and it *is* the tone. Tokens are cheap here at any price. |
| **A second “cheap summarizer model”** | 1:1, one model (fit gate 7). Two models means two voices summarizing one relationship. |
| **Watches as `cron_schedule` + a fetch prompt** | Quiet ticks must not call the Completer. Shipped the other way; do not rebuild it. |
| **Multi-agent / provider orchestration** | Fit gate 7. One Completer. `mcp_enable` changes *this* catalog, not the brain. |
| **Per-turn router LLM** | Second Completer + a new tools subset every bubble (busts prefix cache every message). Prefix enable (Next) is the version that stays hot while in use. |

---

## When something ships

Docs in the same change: env in [readme §4.1](readme.md#41-environment-variables)
when a new var appears; MCP page + `mcp.toml` snippet when the catalog
changes. Then delete the row here.
