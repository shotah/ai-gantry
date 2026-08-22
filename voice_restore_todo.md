# Voice restore

Items 1–6 **shipped**. Do not full-revert to 128k. Later (64k / tone probe /
tiered history) stays below.

Revert ladder if this line was wrong: [todo.md](todo.md#revert-plan-history-32k--filler-strip).

---

## Shipped

1. **Assistant history never stripped.** Last **40** messages verbatim; older
   **user** turns still lose `the`/`a`/`is`. `HISTORY_STRIP_FILLERS=true` stays.
2. **`Voice:` is 8–12 lines**, up to 8 quotes, ledger cap 4k chars.
3. **`/new` distill merges.** Restores dropped quotes. Skips rewrite when there
   is no `Voice:` and no quoted bits in the transcript.
4. **Kernel stamps** (`Header`, `RulesSection`): merge, not flatten; exact
   jokes beat vibe words. Boot / SIGHUP overwrites live `SELF.md` header and
   `PERSONA.md` Self-notes.
5. **PERSONA.md Communication:** tight on **tasks**, not on **chat**. Examples +
   `deploy/persona/PERSONA.md`. **Live `PERSONA.md` is operator-owned — patch by
   hand** (or `make init` only if you accept wiping identity).
6. **`/tokens`** prints `Voice: yes` or `Voice: (none)` next to `summary`.

---

## Watch (live)

- Patch running `PERSONA_DIR/PERSONA.md` Communication to match the example (gitignored).
- If `SELF.md` is vibe-words, prune and put the real nicknames/quotes back.
  Restart or SIGHUP so stamps + persona reload.
- `HISTORY_STRIP_FILLERS=false` is no longer required for grow-back.
- Do not `/new` hoping a flattened `SELF.md` heals. Distill will keep quotes
  it still has; it will not invent the old register.

---

## Later (only if he is still plain)

| Item | When | What |
| --- | --- | --- |
| `HISTORY_MAX_TOKENS=64000` env | Callbacks from last week still miss | Half-rung. Not 128k first. |
| Tone regression probe | Fold or distill prompt changes | [todo.md](todo.md) Later. |
| 64k default in git | 64k env worked for a week | Same files as revert rung 2, `64000`. |
| Tiered history (LLM one-liners) | `/tokens` still says history dominates | [todo.md](todo.md) Later. Not a tone fix. |

---

## Done when

- A nickname from last hour still callbacks after the 40-message tail and after a fold.
- `/new` after a funny session keeps quotes in `SELF.md`.
- `/new` after a bland tool session does not wipe them.
- Chat can riff; tool results stay 2–4 sentences.
