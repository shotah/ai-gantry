# Rosie follow-through — todo

Attack this file. Loop / token work stays in [todo.md](todo.md). Yard UI
stays in [shotah/gantree](https://github.com/shotah/gantree). Vision docs
this list distills:

- [docs/rosie/Gantree_ The Agent That Keeps Looking Out for You.md](docs/rosie/Gantree_%20The%20Agent%20That%20Keeps%20Looking%20Out%20for%20You.md)
- [docs/rosie/AI-Gantry — Rosie Follow-Through Feature Specification.md](docs/rosie/AI-Gantry%20%E2%80%94%20Rosie%20Follow-Through%20Feature%20Specification.md)
- [docs/rosie/Rosie - AI-Gantry — Agent Implementation Missions.md](docs/rosie/Rosie%20-%20AI-Gantry%20%E2%80%94%20Agent%20Implementation%20Missions.md)
- [docs/rosie/PROACTIVITY.md — Rosie Proactive Behavior & Anti-Annoyance Rules.md](docs/rosie/PROACTIVITY.md%20%E2%80%94%20Rosie%20Proactive%20Behavior%20%26%20Anti-Annoyance%20Rules.md)

Status: **next** · **later** · **maybe**
Size: **S** ≈ an afternoon · **M** ≈ a weekend

---

## The concept

The **aim is the user** — who they are, what they want, what is unfinished.
Call that AIM 2.0 if it helps: not a project-plan file, the living model of
one person and the loops they would rather not carry.

```text
notice → remember → schedule → wake → act → follow up
learn who they are → use that to choose well
```

A cron wake is not "send a reminder." Same Rosie, same tools: decide
(message, research, postpone, cancel, or `[silent]`). Spark is not a
side feature — it is how she keeps looking when nobody is talking:
replanning aims, **using live tools**, filling useful personal knowledge,
and showing up with a voice (including a joke when the data earns it).

The 30-day experiment: one agent, one user, one channel. Measure
completed things and *informed* choices (right restaurant, right game),
not message count. Then stop proactive contact and see if they miss her.

Anti-annoyance is load-bearing. Don't dump a questionnaire. A spark
**turn** can still do several things at once (the agent already batches
independent tools). Explicit stop always wins. Silence is valid when
there is nothing useful to say.

---

## Starting bets (not locked)

The vision docs said "don't build a second system." That is a **bias**,
not a law. If a small schema change beats a prompt convention, do the
schema change. If spark is the right place to grow, grow spark.

**Still a good default**

- One agent, one `PERSONA.md`, one `gantry.db`. The relationship is Rosie.
- Reuse memory + cron + MCP before adding a workflow engine.
- Cron already runs `agent.Handle` then `Push`. Keep that as the executor
  unless we learn it is the wrong shape.
- Persona stays readable (2–4k). Put long playbooks in the **spark prompt
  pool** (`internal/cron/spark.go` `DefaultSparkPrompt`) so the standing
  prefix does not become a handbook.

**Open, and currently leaning this way**

| Question | Lean | Why / what would flip it |
| --- | --- | --- |
| Spark vs dedicated follow-up jobs | Spark **is** AIM 2.0. User crons are the *dated* wakes spark (or chat) creates. | Flip if spark's random window is too vague for "Thursday 9am" — those stay `cron_schedule once` |
| Cron ↔ memory mapping | **Yes: optional `memory_id` on the job** (plus subject copied from the row) | Flip if IDs prove too brittle; then subject-only |
| Likes / dislikes | SQLite `preference`, **not** `SELF.md` | Flip only if we decide the user model should always be in the cached prefix (it shouldn't — 4KB + re-billed) |
| Correcting a preference | **Supersede** (keep history), not delete, not `update_self_note` | Flip if we want hard-delete for privacy; `memory_forget` already exists |
| `remove_self_note` | **Don't add it for this.** Wrong store. | Maybe later for *personality* ("stop calling me that"), not sushi |
| Getting-to-know-you | **Spark work**, progressive deepen. One *human* ask if filling a hole; tools can still batch | Flip if it feels like nagging; then slow the pool / `[silent]` more |
| Spark jokes | **Allowed when grounded** (aim + live tool). Ban only the empty check-in | Flip if she starts pep-talking with zero tools |
| Sleep / work hours | **Firm bootstrap** — ask until set; stamp on the turn; skip spark while they sleep | Flip if env `SPARK_*_HOUR` is enough for the experiment |
| New memory kinds | Not needed; subjects carry the taxonomy | Flip if FTS/`/memstats` is unreadable without a `kind=event` |

Harness fit gates in [todo.md](todo.md) (outbound, MCP, Go, 1:1 model)
still apply to *how* we ship. They don't forbid `cron_job.memory_id`.

---

## Already in the harness (audit)

Check the box when you have read the code and know the gap.

| Need | Where it lives | Gap |
| --- | --- | --- |
| Same agent | One process, `PERSONA.md` + `SELF.md`, one `gantry.db`. Cron binds `session_id` / `chat_id` (`delivery.go`) | None |
| User facts | `memory_store` `preference` / `person` / `fact`. Hydrate prefers preference, cap ~30 | Store is **insert-only**. Same subject → two live rows. `Supersede` exists but only the consolidator calls it. No tool to "replace this preference" |
| Agent personality | `self_note` **appends** to `SELF.md` (~4KB). Distill on `/new` says *do not copy facts about the human* | No remove/update tool — by design. Operator prunes. Wrong place for sushi |
| Later turn | `cron_schedule` / `list` / `cancel`. Job is a **prompt string** only | No `memory_id`. Wake hopes FTS + hydration |
| Wake | `runner.go` → `Handle` → `Push` / `[silent]`. `JobUserPrefix` = "execute now" | Prefix does not pin a memory row or say "you scheduled this" |
| `when` | RFC3339, `15:04`, `in 30m`, `every:1h`, spark `qty@HH-HH` | No `tomorrow` / `next Thursday`. Temporal footer can supply ISO dates |
| Spark | `DefaultSparkPrompt` + `SparkPingPrefix` + `sparkToolFirstNote`: aims / cron / live tools. **Explicit "Do not send a joke."** Zero-tool spark is nudged, then `[silent]` | Ban is on *empty* pings. A gym-aim + Garmin + joke is the product, and the prefix currently forbids it. Pool has no user-model lines. Hours are env 6–21 only |
| Don't barge | Spark skip if they messaged recently; `SPARK_START_HOUR` / `END_HOUR`; `[silent]` | No learned sleep/work hours. Work is not DND. Sleep should actually suppress spark |
| World-events | `watch_*` (feeds) | "Waiting for John" = memory + cron (or Gmail), not a watch |

```text
persona     examples/persona/PERSONA.example.md
memory      internal/memory/{memory,tools,builtin}.go   Store insert-only; Supersede unexported to the model
self        internal/selfnote/tools.go                  self_note append-only
spark pool  internal/cron/spark.go                      DefaultSparkPrompt
cron row    internal/cron/store.go                      Job has Prompt, no memory_id
wake        internal/cron/runner.go                     JobUserPrefix
```

---

## Design notes (read these before coding)

### 1. SELF.md vs memory — sushi is not a self-note

`SELF.md` is **who Rosie became** (voice, jokes, rituals, 3–5 north-star
sentences). It is always in the prompt, capped at 4KB, append-only so
the model cannot clobber jokes. Distill already forbids copying facts
about the human into it.

Likes, dislikes, teams, foods, sleep hours, "I wish you could order
groceries" are **who the user is**. They need FTS, correction, and they
must not crowd the persona prefix.

| Store | Tool | Use for |
| --- | --- | --- |
| `SELF.md` | `self_note` | "be warmer", a running joke, a north-star *how I show up* |
| SQLite | `memory_store` `preference` | "loves Thai", "Warriors fan", "sleeps 23:00–07:00" |
| SQLite | `memory_store` `person` | "Mom — birthday Oct 12, likes gardening" |
| SQLite | `memory_store` `fact` / `insight` | events, waiting, aims, `wish/` |
| SQLite | `memory_forget` | "delete that, you got it wrong" / privacy |

**Do not add `remove_self_note` / `update_self_note` for preferences.**
That would teach the model to keep user facts in the 4KB personality
file. If we ever want a SELF.md edit tool, it is for *her* lines
("stop using that nickname"), and it is a separate maybe.

**Correction path (sushi):** do not delete history if we can help it.
`memory_store` on the **same subject** should **supersede** the old row
(`superseded_by` already exists; hydrate/recall already skip those).
Old content stays inspectable in sqlite: "didn't like sushi" → "likes
sushi now (updated 2026-08-24)". `memory_forget` remains for "wipe it."

Category rows beat a row per taco (hydration is 30 slots, preference
sorts first — 30 foods would starve aims):

| Subject | Kind | Content grows by supersede |
| --- | --- | --- |
| `pref/food` | preference | Likes: Thai, tacos. Dislikes: (none). Was: avoided sushi. |
| `pref/activity` | preference | Biking, climbing. |
| `pref/sports` | preference | Warriors. Watched game X on … |
| `pref/hours` | preference | **Bootstrap.** `sleep:` `work:` `quiet:` HH:MM-HH:MM. Firm: footer + skip spark in sleep |
| `pref/proactive` | preference | Nag rules, morning vs evening |
| `person/<name>` | person | Relationship + dates worth knowing |
| `event/<slug>` | fact | The event itself |
| `waiting/<slug>` | fact | Unresolved person/system |
| `follow/<slug>` | fact | Why a cron exists (origin, last action, next check) |
| `aim/<area>` | insight | Months-scale plan (already taught) |
| `wish/<area>` | insight | Capabilities they want her to grow |

Spark deepens a **category**, it does not spray `pref/food/pad-thai`,
`pref/food/taco`, … unless FTS on a single dish proves necessary.

### 2. Cron should point at a memory row

Today the job *is* a blob of prompt text. Future Rosie has to guess a
recall query. A memory id is the straightforward pin.

**Proposal**

- `cron_job.memory_id INTEGER` nullable (no FK — forgotten rows should
  not block cancel).
- Optional `cron_job.memory_subject TEXT` copied from the row at
  schedule time (survives id churn / supersede).
- `cron_schedule` gains optional `memory_id` (and/or `memory_subject`).
  Digests and spark planner rows leave them empty.
- Typical chat flow: `memory_store` → tool result `stored id=42` →
  `cron_schedule` `memory_id=42`.
- On fire: if `memory_id` set, load the row; if `superseded_by` is set,
  walk to the live row; if missing, fall back to `memory_subject` recall,
  then to the prompt alone. Inject a `[job memory]` block so the turn
  does not depend on the 30-row hydrate lottery.
- `cron_list` shows `id=… memory_id=42 subject=follow/passport` so
  dedup is visible.

Spark pings can pass a `memory_id` when they schedule a **dated**
follow-up ("mom's birthday → cron next month pinned to `event/mom-birthday`").
The spark ping itself stays unpinned (it is a gap-scan, not one loop).

### 3. Spark is a full turn (tools + voice), not one action

Spark today only replans `aim/`, and the wrapper **forbids jokes**:

```text
internal/cron/runner.go  SparkPingPrefix   "Do not send a joke, check-in, or pep talk"
internal/agent/agent.go  sparkToolFirstNote / nudge  "Do not send a joke."
```

That was to stop empty presence pings (zero tools, "hey just checking in").
Keep that guard. Drop the blanket ban.

A spark wake is a **normal agent turn**. Independent tools already run in
one response (calendar + Garmin + `memory_recall` + `cron_list`). The
human-facing message can be more than one beat: data, a relevant joke,
one question. What we still don't want is a **questionnaire** (five
getting-to-know-you asks) or a joke with **no** live context.

**Gym aim, time of day** (canonical example — put this in the spark
pool and persona):

```text
11am  recall aim/gym + garmin (today's activity) in one batch.
      Workout logged → [silent] or one short "nice."
      Nothing yet → joke / nudge in her voice ("don't skip the gym")
      grounded in the empty pull. Not a guilt essay.

8pm   same tools. Workout happened → [silent] or a nod.
      Still nothing → disappointed-uncle bit *about the miss*
      (why did the gym not happen?), still one short message.
      Offer help (tomorrow slot, calendar) if that's useful.
      If they already said stop nagging gym → [silent].
```

`[current time]` already has NOW and day-part. Teach her to **shape
the move by the clock**, not to schedule a second spark in the same
turn. The day's other spark pings are the next chances.

**A wake may combine**, in one Handle:

1. Live tools that match aims (Garmin, calendar, mail, search) —
   `mcp_enable` if the prefix is off.
2. User-model: at most **one** hole-filling question (food, team, …)
   if nothing more urgent is on the board.
3. A joke / ritual / uncle-voice **if** it is about what the tools
   just showed (quote SELF.md jokes, don't paraphrase).
4. `[silent]` if the board is fine and there is nothing useful to say.

Ban: joke or pep talk with zero tool calls (keep the existing nudge →
`[silent]`). Allow: Garmin returned empty at 8pm + gym aim → uncle.

Pool lines should include this gym/time-of-day shape, not only "replan
aim/ then silent."

**Progressive deepen** (the one *question*, when that's the move):

```text
pref/food empty     → ask one favorite food; store; [don't dump a list]
1–2 foods           → one more like, or a dislike
3+ foods            → "what have you eaten lately?" OR (if search/maps
                      granted) find a place that fits and offer it
pref/activity empty → what do you like to do
has activity        → one level deeper (team, trail, gym, people)
has a team          → search a recent game; ask if they watched;
                      store the answer on pref/sports
```

Rotate. Recall first. If they were asked this category recently, pick
another or skip the question (tools + gym joke can still run).

Implementation lean: playbook in `DefaultSparkPrompt` + rewrite
`SparkPingPrefix` / `sparkToolFirstNote` so jokes are allowed when
grounded. If she re-asks filled categories, **then** inject
`[user model] food=3 activity=0 sports=1 hours=set` from SQL.

### 4. Sleep / work hours — firmer than food trivia

Hours are not another rotate-with-tacos preference. They gate **when
she is allowed to show up**, and they make gym-at-11am vs uncle-at-8pm
make sense.

**Bootstrap (persona + spark), until set:**

Ask naturally, once, like the PROACTIVITY doc: sleep window, work
window, any other "don't interrupt" blocks. Store `pref/hours`. Do
**not** assume work = DND — they may want gym nags at 11am on a
workday. Sleep is the one we treat as quiet unless they say otherwise.

**Make it firm (lean: two layers):**

1. **Prompt — always visible.** Inject a short `[hours]` line on the
   temporal footer (every turn, not only spark), from the live
   `pref/hours` row. Missing → `hours unknown`. That beats hydrate
   lottery and beats stuffing hours into `SELF.md`. Same trick as
   `[current time]` / `[last pin]`.
2. **Runner — actually skip sleep.** If `pref/hours` has a sleep
   window and `now` is inside it, defer or drop **spark_ping** (and
   examples ping). Explicit user crons ("remind me at 9pm") still
   fire — they picked the time. Env `SPARK_START_HOUR` / `END_HOUR`
   stay the coarse floor (default 6–21) until hours are learned,
   then sleep can tighten further.

`pref/hours` content, keep it structured enough to parse in Go without
a second model:

```text
sleep: 23:00-07:00
work: 09:00-17:00
quiet: (none)          # extra DND they named; not inferred from work
```

Supersede the whole row when they correct it ("I sleep at 22:00 now").

Ask until this row exists (same energy as `aim/bootstrap`). After that,
stop asking; just use it. Food/sports rotate in the leftover spark
budget.

**later** if parsing free text is messy: a tiny `hours` JSON column or
`session_pref` fields (`sleep_start`, `sleep_end`, …) next to
`examples_enabled`. Don't build that until `pref/hours` fails.

---

## Work

### 0. Audit — **done** · S

- [x] Trace `cron_schedule` → row → `Handle` → `Push` / `[silent]`.
- [x] Confirm cron turns hydrate memory and drop prior `[cron]` history
      (`dropCronHistory`). The **pinned row + job prompt** must be enough
      without the original chat.
- [x] Confirm `Supersede` is consolidator-only today; `Store` always
      inserts. **Shipped:** durable `Store` now supersedes same kind+subject.
- [x] List what `PERSONA.example.md` + `DefaultSparkPrompt` already
      teach vs never say (user-model deepen, implicit follow-ups,
      `memory_id`, "not now").

---

### 1. Memory: supersede on same subject — **done** · S

The sushi case. Without this, "I DO like sushi" adds a second live
preference and hydration shows both.

- [x] `memory_store`: if an active row exists with the same
      `kind`+`subject`, insert the new row and `Supersede` the old → new.
      Return the new id. Keep the old row for sqlite history.
- [x] Tool description: "Same subject replaces the live row (history
      kept). Facts about the human go here, not self_note."
- [x] Tests: store sushi dislike; store sushi like on `pref/food`;
      recall/hydrate see only the new content; old id has `superseded_by`.
- [x] `memory_forget` unchanged (hard delete).

**maybe:** `replaces_id` arg if same-subject is too coarse (two facts
under `follow/passport`). Try same-subject first; `follow/<slug>` is
already the uniqueness key.

---

### 2. Cron `memory_id` — **done** · M

- [x] Migrate `cron_job`: `memory_id INTEGER`, `memory_subject TEXT`.
- [x] `cron_schedule` optional `memory_id` (and optional
      `memory_subject` if they did not just store). Validate the id
      exists when given; copy subject from the row.
- [x] `cron_list` prints `memory_id` + subject (truncate prompt as now).
- [x] Runner: resolve live memory (walk `superseded_by`); inject
      `[job memory]` ahead of the job body; log `memory_id` on fire.
- [x] Missing row → log warn, still run on prompt.
- [x] Tests: schedule with id; fire sees content; supersede then fire
      sees new content; deleted id still runs.

Rewrite `JobUserPrefix` to continuation language:

> You scheduled this. If `[job memory]` is present, that is why.
> Recall/tools as needed. Decide: message, act, postpone, cancel, or
> `[silent]`. Do not nag. Original chat may be gone.

Spark/examples prefixes stay their own. Update
`internal/agent/cron_nudge_test.go` if the wrapper text changes.

---

### 3. Spark AIM 2.0: full turn + user-model — **done** · M

- [x] Rewrite `SparkPingPrefix` and `sparkToolFirstNote`: horizon
      work **and** user-model; batch independent tools; joke / uncle
      voice OK when grounded in this turn's tool results + an aim.
      Still `[silent]` when there is nothing useful. **Keep** the
      zero-tool nudge → `[silent]` (that is the empty ping we don't
      want). Delete the blanket "Do not send a joke."
- [x] Expand `DefaultSparkPrompt`: keep aim/cron/live-tool lines; add
      gym/time-of-day (Garmin + `[current time]` → 11am nudge vs 8pm
      miss); add user-model deepen (food, activity→team→game); add
      hours bootstrap if `pref/hours` missing.
- [x] Persona spark example: gym + Garmin + joke, not "one useful
      move then never joke."
- [x] Anti-annoyance: at most one hole-filling *question* per wake;
      skip-recent still applies; "not now" → dated cron ~1h; gym nag
      respects `pref/proactive` / stop.

**later** if she re-asks filled categories:

- [ ] Inject `[user model] food=N activity=N hours=set|missing`.

---

### 3b. Firm hours — **done** · S

- [x] Persona + spark: ask sleep / work / extra quiet **until**
      `pref/hours` exists. Structured content (`sleep: HH:MM-HH:MM`
      etc.). Work ≠ DND.
- [x] Temporal footer `[hours]` from that row (or `hours unknown`).
      Tests on the footer helper.
- [x] Spark/examples runner: if now ∈ sleep window, defer/drop the
      ping (same idea as skip-recent). User-scheduled crons still fire.
- [x] Docs: [cron.md](docs/cron.md) — learned sleep vs `SPARK_*_HOUR`.

---

### 4. Persona: follow-through + getting to know you — **done** · M

One file: `examples/persona/PERSONA.example.md`. Spark pool carries the
playbook; persona carries always/never + a few shots.

Teach:

1. Unfinished things are her job (commitments, dates, waiting, wishes).
   A fact is not automatically a cron.
2. Store user facts as `preference` / `person` / `follow/` — never
   `self_note`.
3. Pin dated work with `cron_schedule` `memory_id`.
4. `cron_list` before a new job; cancel on "already did it" / stop.
5. Wakes decide, including `[silent]`.
6. Getting to know them is **useful inventory**, not chat. Spark (and
   chat, when it comes up naturally) deepens categories over weeks.
7. Hours are bootstrap: get `pref/hours` set; then use `[hours]` /
   sleep skip. Work is not automatically quiet.
8. Anti-annoyance (PROACTIVITY): not now → later; explicit stop;
   scope the no; don't manufacture *empty* pings. A data-backed joke
   is not empty.
9. Calendar / Garmin are tools. Batch them. Findings become memory
   + optional pinned cron.

Examples to add (replace, don't pile on):

```text
"I love Thai food but I'm not into sushi."
  → memory_store preference subject pref/food. Not self_note.

"Actually I do like sushi now."
  → memory_store same subject (supersede). One sentence. Don't lecture.

"Remind me tomorrow to call the dentist." / "I need to renew my passport
next month."
  → store follow/… ; cron_schedule with memory_id; confirm once.

"[cron] Spark of life"
  → In one response: recall aims + pref/hours + cron_list, mcp_enable
    + call live tools that match (Garmin, calendar). Shape by NOW:
    gym aim + no workout + morning → short joke nudge;
    evening → uncle about the miss. Hours unknown → ask sleep/work
    (once). Else at most one user-model question. [silent] if
    nothing useful. A joke with no tools is still wrong.

"Not right now" / "I already did it" / "stop reminding me"
```

- [x] Stay ~2–4k characters (~5.3k; spark pool holds the long playbook).
- [x] Point [docs/persona.md](docs/persona.md) horizon table at
      preference-as-user-model + cron `memory_id`.

---

### 5. Dates — **next** · S / **later** · M

Prefer temporal footer → RFC3339 / `in 30m`. Event-relative = store
`event/<slug>`, compute wake, pin `memory_id`.

- [x] Persona: never pass `when=tomorrow` as a string.
- [ ] **later** only if smoke fails: parse `tomorrow` / weekday /
      `Jan 2` in `ParseSchedule`.

---

### 6. Dedup / completion / idempotent wakes — **done** · S

Easier once `cron_list` shows `memory_id` / subject.

- [x] Persona: same `follow/` or `event/` already on the board → don't
      schedule a twin.
- [x] On fire: if the pinned memory says done / birthday past, cancel
      leftovers, supersede content, `[silent]` or one closer.
- [x] No unique index on prompt text.

---

### 7. Slog — **done** · S

Keep [gantree-contract.md](docs/gantree-contract.md) (`source=cron`).
No new port.

- [x] Fire: `id`, `kind`, `session_id`, `memory_id`, truncated prompt.
- [x] Finish: `push` | `silent` | `error`.
- [x] Tool-call logs already show a follow-up `cron_schedule` in the
      same turn — don't build a workflow tracer.

---

### 8. Tests — **done** · M

Go, next to the package. No LLM in unit tests.

- [x] Supersede-on-store (section 1).
- [x] Cron `memory_id` round-trip + fire inject + supersede walk
      (section 2).
- [x] `[silent]` still skips `Push`; `turnSource` stays `cron`.
- [x] Spark pool parses: aim line, user-model line, gym/time-of-day
      line (`ParseSparkPrompts`). Prefix no longer contains
      "Do not send a joke" as a blanket ban.
- [x] `[hours]` footer: missing vs parsed sleep window.
- [x] Spark skip when `now` is in sleep (runner test).
- [ ] Dates only if `ParseSchedule` grows.

Persona behavior is a live smoke (stdio/Telegram), not a stub Completer.

| Spec story | Go can assert | Persona / spark |
| --- | --- | --- |
| Explicit reminder | Job + `memory_id` + `[job memory]` on fire | She stored + scheduled |
| Implicit passport | — | Persona |
| Continuation | Pinned row in the turn | She decides |
| Completion | Cancel + supersede | She called them |
| No duplicate | List shows subject | She checked |
| Thai now, sushi later, sushi yes | Hydrate = one `pref/food` | She stored twice |
| Spark food deepen | Pool contains the line | She asks at most one |
| Gym 11am / 8pm | Prefix allows grounded jokes | She pulled Garmin |
| Hours bootstrap | Footer + sleep skip | She asked until set |

---

### 9. 30-day experiment — **later** · S

- [ ] One crane, one user, Telegram, updated persona, spark on.
- [ ] Grant search + Google as needed.
- [ ] Day 30: `/engagement off` and `/examples off` (or `EXAMPLES_QTY=0`); decide whether dated
      user crons stay. No guilt message.
- [ ] Look at `pref/*` richness, `follow/` completions, slog `source=cron`.

---

## Definition of done

**Follow-through:** "Mom's birthday is October 12, I always forget." →
`event/` + `follow/` stored, cron pinned to that id, later the same
process wakes with `[job memory]`, she helps or `[silent]`, then stops
when it's handled.

**User-model:** Over days, spark (and chat) fills `pref/food`,
`pref/activity`, maybe a team, and **`pref/hours`**. Then "where should
we eat?" uses food prefs. Spark does not fire at 2am if they sleep
23:00–07:00.

**Spark as a presence with a reason:** Gym is an aim. 11am spark pulls
Garmin + calendar in one batch, jokes about not skipping. 8pm spark
pulls Garmin, sees no workout, disappointed uncle — not a blank
"checking in," not a silent skip just because jokes used to be banned.

All of that is the same aim: **the user**.

---

## Attack order

1. Audit (0).
2. Supersede-on-store (1) — unblocks sushi without a SELF.md tool.
3. Cron `memory_id` + wake inject + slog (2, 7, 8).
4. Spark prefix/nudge (drop blanket joke ban, keep zero-tool silent) +
   pool (gym/time-of-day, user-model) + hours footer/sleep skip (3, 3b).
5. Persona cut (4) + dedup/dates (5–6).
6. Live smoke: birthday, food deepen, sushi correction, gym 11am vs 8pm.
7. Gap-count prefix only if she re-asks full categories.
8. Experiment checklist (9) when someone is onboarded.

---

## When something ships

Update [docs/persona.md](docs/persona.md) / [docs/cron.md](docs/cron.md)
/ [docs/memory.md](docs/memory.md) in the same change. Check the box
here. This file stays the attack list; don't add a fifth manifesto
under `docs/rosie/`.
