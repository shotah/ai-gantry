# ai-gantry — compression todo

**Token-reduction ideas that must not cost tone.** The fear is real: the old
summarizer was told to *"Drop chitchat"* — which is exactly where the jokes
live. So this list is split by what each idea can lose, and every lossy idea
has to answer "where does the tone go instead?"

Same rules as [future_todo.md](future_todo.md): deleted ideas stay deleted,
fit gates apply (native Go, MCP or nothing, tiny catalog). Schema slimming
stays **rejected** — see the "Not doing" row there; not relitigated here.

Status: **next** · **later** · **maybe** · **shipped**
Size: **S** ≈ an afternoon · **M** ≈ a weekend

---

## The frame: three different "token costs"

Not all tokens cost the same. Every idea below should say which bucket it cuts.

| Bucket | What | Cut it by |
| --- | --- | --- |
| **Billed prompt tokens** | Cloud APIs re-bill the whole prompt every turn (cached prefix is discounted, not free) | Smaller persona / history / hydration / schemas |
| **Prefill latency** | Local models pay wall-clock for every *uncached* token (`volatile_est_tokens` predicts `first_token_ms`) | Byte-stable prefix; less churn, not just less text |
| **Context ceiling** | Small local models drown past ~8–32k regardless of cost | Bounded everything (mostly done) |

Corollary: **churn is a cost even when size isn't.** A 2k-char summary that
rewrites itself every fold invalidates the KV cache for everything after it.
Sometimes the win is *stop rewriting*, not *write less*.

---

## Where the tokens actually are (measure first)

Per-turn logs already carry `prompt_est_tokens`, `volatile_est_tokens`,
`schema_est_tokens`, `hydration_est_tokens`. Current standing costs:

| Source | Today | Bound |
| --- | --- | --- |
| Persona (SOUL+RULES+USER+TOOLS) | ~14 KB ≈ ~3.5k tokens after the diet (was ~19 KB) | none |
| SELF.md | ≤ 4 KB ≈ ~1k tokens | capped |
| Session summary | ≤ 2000 chars | capped, but churns |
| History | 200 msgs / 128k est tokens | huge default |
| Memory hydration | ≤ 30 rows | capped |
| Tool schemas | per-manifest; `est_tokens` logged at boot | curation only |
| Tool results in-loop | 6000 chars each; collapse older than last 2 (args stubbed too) | capped |

### `/tokens` breakdown command — **shipped**

`/tokens` prints the table above for *this* session: persona / summary /
history / hydration / schemas, est tokens each. Also in `/help`, Telegram
`setMyCommands`, and the stdio banner. Use it before justifying more cuts.

---

## Tone preservation (do these before any lossy compression)

### 1. Fix the summarizer prompt — it's told to delete the jokes — **shipped**

`summarizeSystem` in `internal/session/summary.go` now keeps voice: running
jokes, nicknames, coined phrases, and the current game. Quote a joke's exact
wording — a paraphrased joke is a dead joke.

### 2. Extractive "greatest hits," not abstractive paraphrase — **shipped**

Same prompt: keep up to 3 short verbatim quotes (~100 chars) when they carry
a running bit. Bound by the existing 2000-char summary cap.

### 3. Tone ledger — a 2–4 line voice block in the summary — **shipped**

Same `session.summary` string, two labels. Fold prompt maintains both;
`ensureVoiceLedger` copies prior `Voice:` forward if a small model drops it.

```text
[session summary]
Facts: … (the current paragraph)
Voice: dry; running gag: "that gull had a mortgage"; he is "Chef" this week.
```

Facts churn. Voice copies unchanged unless a new joke / nickname / game
landed. On `/new`, `Voice:` is handed to the `SELF.md` distill and `Facts:`
park as a memory episode (consolidator splits them). `USER.md` is never
written.

### 4. Distill-to-SELF on trim, not only on `/new` — **later · M**

Long-running sessions can trim for weeks without ever hitting `/new`, so the
distiller (`internal/agent/self.go`) never runs and durable personality never
escapes the dying turns. When a fold drops turns containing tone-worthy
material, let the fold *also* emit an optional one-line `self_note` (the
plumbing exists; the tool already refuses past the 4 KB cap). Tone graduates
from history (expensive, doomed) to SELF.md (capped, permanent). This is the
real answer to "compression loses the funny": don't store the funny in the
compressible layer.

### 5. Tone regression probe — **later · S**

"Without fidelity loss" is testable, cheaply. A `make` target: take a fixture
transcript with a planted running joke, fold it through the summarizer, then
ask the model (fresh context + summary) to make the callback. Grade
pass/fail with one LLM call. Run it whenever a summary/distill prompt
changes. Not CI-blocking — a smoke test so prompt edits don't silently
lobotomize the humor.

---

## History compression (the algorithm part)

### 6. Tiered history: verbatim → one-liners → paragraph — **later · M**

Today history is binary: full verbatim until trimmed, then melted into one
paragraph. Add a middle tier. Oldest = rolling summary (exists). Middle =
one line per turn (`user asked X / agent did Y, joked Z`). Recent = verbatim.
Tone degrades gradually instead of falling off a cliff, and the middle tier
compresses ~10:1. This is the "some algorithm" you were reaching for, and it
lives entirely in `internal/session` + the existing summarizer. Do it only
if `/tokens` says history dominates (default bounds are so wide it probably
does on long sessions).

### 7. Strip tool-call plumbing from aging history — **shipped**

In-turn: tool results older than the last 2 already collapsed; matching
assistant **argument JSON** is now stubbed too (`{}`), with Gemini
`thought_signature` kept in `Raw`. Session history never stored tool
payloads (user/assistant text only) — the "week-old `route_eta` coords"
were an in-turn problem, not a cross-turn one.

Docs now match the code (`keepRecentToolResults = 2`, not 4).

### 8. Append-only summary epochs (churn killer) — **maybe · M**

Each fold rewrites the whole summary → invalidates the cached prefix for
everything after it → local prefill repays the entire history. Alternative:
summary as append-only epoch paragraphs (fold *adds* a paragraph; a rare
compaction pass merges old epochs). Prefix stays byte-stable across most
folds. Cuts prefill latency, not billed size — worth it mainly on local
models. Measure with `first_token_ms` before and after; skip if folds are
rare in practice.

---

## Standing-cost trims (no algorithm, just diet)

### 9. Persona prose pass — **shipped**

RULES.md (~7 KB → ~4.4 KB) and TOOLS.md (~7.2 KB → ~4.4 KB deploy / ~6.5 KB
examples). Overlapping rules merged, self-notes dropped from TOOLS (already
in RULES), recipes tightened. Tone still lives in SOUL/SELF. Roughly ~1.3k
tokens/turn off the standing prefix. Further cuts only if `/tokens` still
says persona dominates.

### 10. Right-size the history defaults — **maybe · S**

`HISTORY_MAX_TOKENS=128000` means the summarizer basically never runs until
the context is enormous — you're paying full verbatim price for ancient
turns that a good summary (post-idea-1/2/3) would carry fine. Once the tone
fixes are in and probed (idea 5), drop the default (or document ~32k as the
recommended setting). Explicitly sequenced *after* the tone work: lowering
the bound today feeds more turns into a summarizer that has not been
probe-tested for tone (idea 5).

### 11. Hydration dedup vs summary/SELF — **maybe · S**

The `[memory]` block re-states things the summary or SELF.md already carry
("user prefers no fluff" in both = paid twice). Cheap version: hydration
skips rows whose content substring-matches the current summary/SELF text.
It's ≤30 rows, so the ceiling is small — check `hydration_est_tokens` first;
only worth doing if it's routinely fat.

---

## Not doing

| Idea | Why not |
| --- | --- |
| **LLMLingua / perplexity-based token pruning** | Needs a Python/torch sidecar (fit gate 3: no JIT), and it deletes "low-information" tokens — which is precisely what a joke looks like to a perplexity filter. The named-algorithm answer to "compress the communications" is the one that eats tone first. |
| **Embedding-based semantic dedup / retrieval summaries** | Already rejected for memory ([choices](docs/choices.md)); same reasons — second model, opaque, unfixable when wrong. FTS5 + prompts is enough at one-user scale. |
| **Schema slimming** | Rejected in [future_todo.md](future_todo.md) — the bytes are the tool manual. Lever remains publishing fewer tools (`tools` / `exclude` / `--tool-tier`). |
| **Compressing SELF.md harder** | It's 4 KB, capped, and it *is* the tone. This file is the one place tokens are cheap at any price. |
| **A second "cheap summarizer model"** | 1:1, one model (fit gate 7). Two models means two voices summarizing one relationship. |

---

## Build order

| Step | What | Why first |
| --- | --- | --- |
| 1 | `/tokens` readout (idea 0) | **shipped** |
| 2 | Summary prompt tone fixes (ideas 1–2) | **shipped** |
| 3 | Tool-arg collapse + doc fix (idea 7) | **shipped** |
| 4 | Persona diet (idea 9) | **shipped** |
| 5 | Tone ledger (idea 3) | **shipped** |
| 6 | Tone probe (idea 5) | Gate before lowering history defaults |
| 7 | Distill-on-trim / tiered history (4, 6) | The real machinery, now safer to build |
| 8 | Defaults + churn work (8, 10, 11) | Only if `/tokens` says so — 10 is the next real cut |

When something ships: docs in the same change, then mark **shipped** here —
same contract as [future_todo.md](future_todo.md#when-something-ships).
