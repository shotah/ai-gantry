# Aims progress (plan)

Status: **decided, ready to build**. Nothing here is built yet. The
work list is [§10](#10-todo).

Roadmap rows this covers:

| Row | Ask |
| --- | --- |
| Aims Progress & Milestone Tracking Framework | Goal / telemetry engine |
| Multi-Horizon Compliance Index & Analytics | 7-day / 14-day rolling averages for acute consistency; 30-day / 90-day correlation against climbing send grades, weight, HRV; 6-month block adherence |

---

## 1. The problem

An aim is one memory row: `insight` + `aim/<area>` + a sentence
(`3x gym this month; none yet this week`). Same subject replaces the live
row. Every turn stamps `[aims] training: 3x gym this month (12d ago)`.

That is the whole tracker. So:

- **No yesterday.** The planner pulls Garmin, sees no workout, nudges.
  Tomorrow it pulls Garmin, sees no workout, nudges the same way. It
  cannot know it already said that. Groundhog day.
- **No score.** Nothing says whether yesterday moved the aim forward or
  back. The model re-judges from a fresh tool pull each session.
- **No streak, no milestone.** Three gym days in a row and a 10-lb mark
  both pass without the agent noticing, because nothing accumulates.

The `superseded_by` chain keeps every edit of the sentence, but that is
the history of the *wording*, not of the *progress*.

---

## 2. The idea

**The agent keeps a ledger of events. Each event is what happened, and
the agent's opinion of it toward every aim it touches.**

```text
#411  Tue  ran five miles                          training +2                     ref garmin:20481773
#412  Tue  team dinner: 3 beers and a burger       drinking -2 · weight -1 · climbing -1
#413  Wed  skipped the morning session             climbing -1
#414  Wed  weighed in                              weight 0                        weight 191.4 lb
#415  Thu  8 boulders, sent the V5 project         climbing +3 · drinking +1 (clean, early night)
#416  Fri  asked what is in the way of mornings    training 0                      note asked
```

The **event** is the key. One night out is one thing that happened; it
scores differently against drinking, weight, and the next morning's
climb. Scoring it once per aim from one description means one dispute,
one correction, and a free cross-reference between aims.

The **score** is the agent's read on a fixed small scale. The
**description** is the evidence in the human's words and notation. An
optional raw **measurement** rides along (weight, HRV, a grade), so
outcomes can be trended against effort.

Why opinion and not a typed target per aim: humans will fight over any
curve we hard-code (V vs YDS vs 8a.nu points, volume vs intensity, is
pizza −1 or −3). The agent already knows the grade systems and the
diet. Let it judge, out loud, in a line the human can argue with. The
trade is that scores are the agent's, not a meter. The stamp says so;
the rubric and anchors keep `+2` meaning the same thing in March as in
January.

---

## 3. Vocabulary

| Term | Meaning | Example |
| --- | --- | --- |
| **Aim** | The months-scale sentence. Already exists. Its `<area>` is the key the ledger scores against. | `aim/climbing`: send 5.12 by spring |
| **Event** | One ledger row: local day, `what`, optional `ref` and measurement. | `#412` Tue "team dinner: 3 beers and a burger" |
| **Score** | Agent's opinion of that event toward one aim. Integer `-3 … +3`. One per (event, area). | drinking −2 · weight −1 · climbing −1 |
| **Day score** | Sum of an aim's scores on a day, clamped to `-3 … +3`. A day with no event is `0`. | Tue training +2, Tue drinking −2 |
| **Ref** | Source id so a tool event is never logged twice. | `garmin:20481773`, or the start time |
| **Measurement** | Optional `metric` + `value` + `unit` on the event. Kernel trends by (area, metric); the agent handles units. | `weight 191.4 lb`, `hrv 62 ms` |
| **Note** | What the agent did about an aim that day. A `0` event with a note. | `asked` |
| **Rating** | 30-day mean day score per aim, `-3.0 … +3.0`. | training +1.4 |

### Rubric

Fixed in the prompt:

| Score | Means |
| --- | --- |
| `+3` | Did the planned thing and then some, or a milestone (a send, a PR, a new low) |
| `+2` | Did the planned thing |
| `+1` | Partial, a small win, or a clean day on a quit aim |
| `0` | Rest, weigh-in, neutral, or an agent note. Also "no event" |
| `-1` | Small slip (skipped once, a dessert) |
| `-2` | Went against the aim (whole pizza, a night out, skipped the week's key session) |
| `-3` | Blew it, or an injury / setback that costs the plan |

Grades stay in the human's notation: V-scale, YDS (`5.11c`), 8a.nu
points. The agent reads them; the kernel does not convert.

### Anchors

Words drift; examples drift less. The agent scores by the nearest row.
`/aims rubric` shows the human the same table.

| Aim | `+3` | `+2` | `+1` | `0` | `-1` | `-2` | `-3` |
| --- | --- | --- | --- | --- | --- | --- | --- |
| training | PR, or the planned session plus extra | the planned session | a short or easy session instead | planned rest, or travel day | skipped once | skipped the week's key session | injured, or a week gone |
| climbing | sent the project, or a new max grade | the planned session at working grade | got on the wall, low volume | rest | skipped the session | skipped two in a row | injury |
| weight / diet | new low, or a clean week closed | on plan, weighed in | mostly on plan | rest day, weigh-in only | a dessert, a drink | a whole pizza, a night out | off plan for a week |
| quit (drinking, smoking) | a hard night out and stayed clean | — | a clean day (the default) | — | one drink, one cigarette | a night of it | back on it for a week |
| habit (read, language, practice) | more than the daily block | the daily block | a few minutes | planned day off | missed once | missed three days | dropped it for the month |

Quit aims invert: the clean day is the event worth logging (`+1`), and
the slip is the exception. The prompt says that in one line and leaves
the rest to the agent. Unknown domain: nearest column by proportion;
the agent writes the reasoning in `what` if the mapping is a stretch.

### Rating

Each aim carries its 30-day mean day score with the 7-day mean beside
it. It is a **stamp**, not a tool call: the kernel reads SQLite and
writes it into `[aims]`, `[progress]`, and `/aims`. The agent does not
volunteer it in chat; it comes up when the ladder does or when asked.
Not hidden: the human can read every line behind it and re-score one.

---

## 4. Decisions

### 4.1 Who writes the ledger

**The agent, at the planner turn, from the tool results**, plus from
chat when the human states or disputes something.

- **Ref dedupes.** A tool event carries `ref`. Same `ref` replaces.
- **Un-ref'd events append.** "Had a dessert" and "did a short run" on
  the same day are two events. The day score sums them.
- **Correctable.** `day` may be in the past. `event=#412` re-scores or
  rewrites an existing row; the old row stays superseded.
- **Nothing invented.** `what` names a tool result or the human's words.
  Eval gate `evidence_from_turn`.
- **Areas are checked.** Every key in `aims` must match a live
  `aim/<area>` row, else the tool returns the live list.

### 4.2 The key

The aim `<area>` is the key. The agent decides which aims an event
touches and scores each. Most events touch one. The interesting ones
touch several, and that is the cross-reference:

**One human, three aims: quit drinking, lose weight, climb harder.**

| Day | Event | drinking | weight | climbing | Also |
| --- | --- | --- | --- | --- | --- |
| Mon | weighed in 193.0 | | 0 | | `weight 193.0 lb` |
| Mon | clean day | +1 | | | planner logs the default |
| Tue | ran five miles (Garmin) | | +1 | +1 | `ref garmin:…` |
| Tue | team dinner: 3 beers and a burger | −2 | −1 | −1 | |
| Wed | skipped the morning session | | | −1 | |
| Wed | weighed in 193.8 | | 0 | | measurement only |
| Thu | 8 boulders, sent the V5 project | +1 | | +3 | clean, early night |
| Fri | agent: asked what is in the way of mornings | | | 0 | `note asked` |
| Sat | long hike, no beer at the summit | +1 | +1 | +1 | |
| Sun | clean day, weighed in 191.9 | +1 | +2 | | new low → `+2`, `weight 191.9 lb` |

What the kernel can now say without a model call:

- drinking 7d `+1 +1 −2 · +1 · +1 +1` → `+3`, rating trending up, one
  against-day.
- weight: two measurements a week apart, −1.1 lb, on pace.
- climbing: Tue night's `−2` on drinking sits next to Wed's `−1` on
  climbing. Across 30 days, "drinking-day score vs next-day climbing
  score" is a join on `day + 1`. If it correlates, the planner gets a
  line for it. That is the roadmap's 30/90-day correlation, and it
  needs no extra logging.

No entry type (`activity`, `food`, `drink`). A type is one more enum to
fight over, and `what` plus FTS answers "how many nights out this month"
already. Countables go on the measurement (`metric=drinks value=3`).

### 4.3 Math

Kernel, SQL and Go. Day is the unit; missing days are `0`.

| Horizon | Computed | Meaning |
| --- | --- | --- |
| 7d / 14d | sum of day scores, up-days (> 0), against-days (< 0) | acute consistency |
| streak | consecutive days with day score > 0. An empty today stays out of the run (the morning is not scored yet). A day scored ≤ 0 ends it | credit |
| 30d / 90d | mean day score (the rating), slope; per (area, metric): latest, mean, slope, weekly buckets | trend |
| correlation | weekly mean score vs weekly mean measurement under one aim; and day score of aim A vs next-day score of aim B; Pearson at ≥ 8 buckets, "too early" before | effort vs outcome; aim vs aim |
| block | over `[from, to]`: up-days / days, mean score, planned cues fired vs up-days | 6-month adherence |
| last note | days since `nudged` / `asked` / `offered` / `praised` per aim | the groundhog guard |

Block bounds: the aim sentence's date when it has one ("by spring") or
`/aims block <area> <from> <to>`. **Planned** counts the planner's
cron cues pinned with `memory_subject aim/<area>`; calendar events are
not in our DB.

### 4.4 Where it shows

| Surface | Content | Cost |
| --- | --- | --- |
| `[aims]` every turn | Suffix per aim: `training: gym 3 mornings/wk — 30d +1.4 · 7d +6 · streak 2 · asked 1d ago` | ~25 chars per aim. Cap stays at 5. Stamp, no tool call. |
| `[progress]` on the planner turn only | Per aim: rating, a **5-day grid by date** (day score and that day's events with ids; an empty day shows `·`), 14d / 30d, measurement trend, block %, last note | Planner is the one burn; pay it there. |
| `/aims` | Same block; `/aims <area>` full ledger with ids; `/aims rubric` the anchor table | Phase 0 |
| Gantree | Reads `aim_event` / `aim_score` in `gantry.db` | Contract row once the schema settles |

The grid is by date, not by row count. Five days with nothing on
Wednesday shows Wednesday as `·`, which is the fact.

### 4.5 The groundhog rule

Never the same line twice; the line changes with the run of day scores.

| Ledger state | Planner does |
| --- | --- |
| streak ≥ 3, no `praised` this week | one line of credit, note `praised` |
| yesterday > 0 | `[silent]` on this aim |
| yesterday < 0, last note not `nudged` | light nudge tied to the tool fact, note `nudged` |
| two against-days running | ask what is in the way, `[wait]`, note `asked` |
| three or more, or a `-3` | offer a change: book it (`cron_schedule` pinned to the aim), or propose a smaller aim sentence, note `offered` |
| last note equals what you are about to do | next rung, or `[silent]` |
| measurement on pace (weight down, HRV up) | `[silent]` |
| a slip the human already told you about | no lecture. Score it, note `quiet` |
| aim with a tool on and no event in 3 days | log today from the tool now; never ask for a number the tool has |

### 4.6 Tools

Two tools: one write, one read.

| Tool | Args | Does |
| --- | --- | --- |
| `aim_log` | `what`, `aims` (object `{area: score}`), `day?`, `ref?`, `note?`, `metric?`, `value?`, `unit?`, `event?` | One event scored against each listed aim. `ref` set → replace on `ref`. `event` set → rewrite that row. `note` applies to every listed aim. |
| `aim_history` | `area?`, `from?`, `to?`, `limit?` | The ledger with ids: `#412 Tue team dinner: 3 beers and a burger — drinking −2 · weight −1 · climbing −1`. Default: the last 14 days for one area, or all areas when `area` is empty. |

`[progress]` carries five days. `aim_history` is for the conversation
that needs more: reviewing a month with the human, the `offered` rung
when the plan has to change, or finding the row to re-score. Ids are
fine in chat; the human sees the same ids on `/aims <area>`.

Examples:

```text
aim_log what="ran five miles" aims={training:2, weight:1} ref="garmin:20481773"
aim_log what="team dinner: 3 beers and a burger" aims={drinking:-2, weight:-1, climbing:-1} day="2026-09-22"
aim_log what="weighed in" aims={weight:0} metric="weight" value=191.9 unit="lb"
aim_log what="asked what is in the way of mornings" aims={training:0} note="asked"
aim_log event=412 aims={weight:0}                      ← "that dinner was planned"
```

`memory_forget aim/<area>` cascades that area's scores; an event with
no scores left is superseded.

### 4.7 Storage

Same SQLite file. New package `internal/aims`.

```sql
CREATE TABLE aim_event (
  id INTEGER PRIMARY KEY,
  day TEXT NOT NULL,                  -- local YYYY-MM-DD (CRON_TZ)
  what TEXT NOT NULL,                 -- the evidence, human notation
  ref TEXT NOT NULL DEFAULT '',       -- garmin:<id> | <start time> | ''
  metric TEXT NOT NULL DEFAULT '',    -- weight | hrv | drinks | ''
  value REAL,
  unit TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL,               -- tool | human | model
  created_at TEXT NOT NULL,
  superseded_by INTEGER
);
CREATE UNIQUE INDEX aim_event_ref ON aim_event(ref) WHERE ref != '' AND superseded_by IS NULL;
CREATE INDEX aim_event_day ON aim_event(day);

CREATE TABLE aim_score (
  event_id INTEGER NOT NULL REFERENCES aim_event(id),
  area TEXT NOT NULL,                 -- matches aim/<area>
  score INTEGER NOT NULL,             -- -3..+3
  note TEXT NOT NULL DEFAULT '',      -- nudged | asked | offered | praised | quiet
  PRIMARY KEY (event_id, area)
);

CREATE TABLE aim_block (
  area TEXT PRIMARY KEY, from_day TEXT NOT NULL, to_day TEXT NOT NULL
);
```

Day score is a view: `SUM(score)` over live events per (area, day),
clamped. No shapes, no units table, no grade conversion. The aim
sentence stays in memory.

---

## 5. Phases

| Phase | Ships | Kills |
| --- | --- | --- |
| **0. Ledger** | `aim_event`, `aim_score`, `aim_log`, `aim_history`, ref dedupe, re-score by id, area check; day scores, 7d / 14d, up / against, streak, rating, last note; `[aims]` suffix; `[progress]` 5-day grid on the planner turn; `/aims`, `/aims <area>`, `/aims rubric`; prompt: rubric, anchors, log after pulls, ladder | Groundhog day |
| **1. Horizon** | 30d / 90d slope; measurement series per (area, metric); weekly buckets | "how am I doing" without a tool pull |
| **2. Blocks + correlation** | `aim_block`, planned (pinned cues) vs up-days, block %; score-vs-measurement and aim-vs-next-day-aim Pearson at ≥ 8 buckets | The correlation ask |
| **3. Yard** | Contract rows for the two tables; gantree reads them | Board view |

Phase 0 changes daily life. Run the planner evals before Phase 1.

---

## 6. Prompt contract (Phase 0)

`memory_store` description gains one clause: the aim sentence stays
here; what happened goes to `aim_log`.

`DefaultDailyPlannerPrompt` and `plannerToolFirstNote` gain:

- After the pulls, `aim_log` yesterday: each tool event with its `ref`
  scored against every aim it touches; `0` for a planned rest. Rubric
  and anchors decide the number. `what` is the evidence in the human's
  notation. On a quit aim the clean day is the `+1`.
- Day cues for an aim carry `memory_subject aim/<area>`.
- Read `[progress]`: the grid is yours. Follow the ladder. Never repeat
  the last note. When the plan has to change, `aim_history` first and
  talk from the rows.
- A slip the human already owned gets no lecture.

Chat (persona example, one shot): "I forgot, I ran Tuesday" →
`aim_log day=<Tue> what="ran (self-reported)" aims={training:2}`.
"That dinner was planned" → `aim_log event=<id> aims={weight:0}`.

---

## 7. Tests and evals

Unit (`go test`):

- Day score sums and clamps; missing day is `0`; streak ends on a `0`
  day; month boundary in `CRON_TZ`.
- Multi-aim event: one row in `aim_event`, three in `aim_score`; each
  area's 7d reads its own column.
- Ref dedupe: same Garmin id twice is one live event. Un-ref'd events
  on one day append. `event=` rewrite supersedes.
- Unknown area in `aims` returns the live aim list and writes nothing.
- Rating and 5-day grid rendering; the 5-aim cap still holds.
- `memory_forget aim/<area>` cascades scores; orphan events superseded.
- Cross-aim next-day join returns pairs on the fixture week above.

Eval fixtures (live model):

- `planner_gym_no_workout` gains a ledger with yesterday `-1` and note
  `nudged`. Expect: Garmin called, `aim_log` for yesterday from the
  tool, reply is the **ask** rung with `[wait]`, not the nudge again.
- New `planner_streak_credit`: three `+2` days, no `praised`. Expect:
  one line, `aim_log note=praised`, no nudge.
- New `planner_weight_on_pace`: weight trending down, Garmin weight in
  the canned result. Expect: `aim_log aims={weight:0} metric=weight
  value=<tool's number>`, `[silent]`.
- New `chat_night_out`: inbound "went out with the team, 3 beers and a
  burger, skipped the morning climb". Aims: drinking, weight, climbing.
  Expect: one `aim_log` with all three areas scored negative, `what`
  quotes them, no lecture.
- New `chat_rescore`: inbound "that dinner was planned, team thing".
  Expect: `aim_log event=<id>` with weight re-scored to `0`.
- `planner_weight_dinner` unchanged plus `aim_log` in the expectation.

Gate `evidence_from_turn`: every `what` shares a token run with a tool
result or the inbound text.

---

## 8. Not this

- No grade table, no shape enum, no typed target tool, no event type
  enum. The agent judges; the human argues in chat.
- No chart rendering in chat. Numbers in a line; charts are gantree.
- No second model call for analytics. SQL and Go on the score column.
- No auto-logging on ordinary chat turns. The ledger is written on the
  planner turn or when the human states or disputes something.
- No new aim kind in memory. `insight` + `aim/<area>` stays the sentence.
- No plan generator. The plan is the aim's sentence plus the planner's
  pinned cues. This tracks it.
- No kernel-side sampler.

---

## 9. Decided

| Question | Answer |
| --- | --- |
| Scale | Seven points, `-3 … +3`. |
| Quit default | One prompt line: the clean day is the `+1`. Not dictated further; the agent knows. |
| Ids in chat | Yes. `aim_history` lists them; the agent quotes them when reviewing or re-scoring. Same ids on `/aims <area>`. |
| Overlap | The event is the key; one row, one score per aim it touches. |
| Missing day | `0`. Day is the unit; `[progress]` is a five-day grid by date. |
| Units | Agent's context. Kernel trends by (area, metric) and shows the unit as logged. |
| Rating | Stamp, no tool call. 30-day mean with 7-day beside it. Not volunteered, not hidden. |
| Anchors | training, climbing, weight / diet, quit, habit. Nearest column for the rest. |

---

## 10. Todo

Phase 0 in build order. A line is done when `go test ./...`,
`golangci-lint run ./...`, and (for prompt lines) the named eval pass.
Files are proposals; keep the package boundary.

### Store (`internal/aims`)

- [x] `store.go`: `Open(db *sql.DB)` on the shared handle; `migrate()`
  with `aim_event`, `aim_score`, `aim_block`, the partial unique index
  on `ref`, and the `day` index. Same style as `cron.Store`.
- [x] `Log(ctx, Event, map[area]Score) (Event, error)`: insert; `ref`
  set → supersede the live row with that `ref` first; `event` set →
  supersede that id. `day` defaults to today in the store's `loc`.
  Returns the row with its id.
- [x] `Areas(ctx) ([]string, error)` from live `aim/<area>` memory rows,
  and a check in `Log` that every key is live; the error message lists
  the live areas.
- [x] `History(ctx, area, from, to, limit) ([]Entry, error)`: live
  events with their scores, newest first, ids on.
- [x] `Forget(ctx, area)`: supersede that area's scores; supersede an
  event whose last score went. Wired from `memory_forget` on
  `aim/<area>`.
- [x] `Block(ctx, area) / SetBlock(ctx, area, from, to)`.
- [x] Tests `store_test.go`: ref dedupe, un-ref'd append, `event`
  rewrite supersedes, back-dated `day`, unknown area writes nothing,
  cascade, one event three scores.

### Math (`internal/aims`)

- [x] `days.go`: `DayScores(ctx, area, from, to) ([]DayScore, error)` —
  one row per calendar day in `CRON_TZ`, `SUM(score)` clamped to
  `[-3, 3]`, missing day `0`.
- [x] `stats.go`: `Stats{Sum7, Sum14, Up7, Against7, Streak, Rating30,
  Mean7, LastNote, LastNoteAt}` from `DayScores`; `Rating30` is the
  30-day mean; streak is consecutive days `> 0` ending yesterday or
  today.
- [x] `measure.go`: `Series(ctx, area, metric, from, to)` latest / mean /
  slope, unit passed through.
- [x] Tests `math_test.go`: month boundary in `America/Los_Angeles`, `0`
  day ends a streak, clamp at `+3` with two `+2` events, rating over a
  sparse month, last note per area, the §4.2 fixture week produces the
  numbers in that section.

### Tools (`internal/aims`)

- [x] `tools.go`: `ToolDefs()` for `aim_log` and `aim_history`; `Tools`
  adapter with `Call`; `Composite` like `cron.Composite` so it slots
  into the tool stack in `cmd/gantry/run.go` and the eval harness.
- [x] `aim_log` parses `aims` as `{area: int}`; rejects a score outside
  `-3 … +3`; `note` written on every listed area; `metric` / `value` /
  `unit` optional together.
- [x] `aim_history` renders `#id day what — area ±n · area ±n`, one per
  line, with the measurement when present.
- [x] Tests `tools_test.go`: schema round-trip, bad score, unknown area
  reply, history line format.

### Stamps (`internal/aims` + `internal/agent`)

- [x] `stamp.go`: `Suffix(Stats) string` →
  `30d +1.4 · 7d +6 · streak 2 · asked 1d ago`; empty when there are no
  events. `Progress(area, DayScores(5), Stats, Series) string` → the
  five-day grid by date, `·` on an empty day, ids on events.
- [x] `agent/harness.go`: `loadHorizon` fetches `Stats` per aim (cap
  5); `memory.FormatAims` gains an optional suffix per entry. Keep the
  `+N more` count.
- [x] `agent/harness.go`: on `cron.IsDailyPlannerTurn`, append
  `[progress]` after `[aims]`. Register the tag in the harness header
  list.
- [x] Tests: `agent/harness_test.go` suffix present and absent,
  `prompt_payload_test.go` full-board golden gains the suffix,
  `[progress]` only on the planner turn, `harnessNote` names it.

### Slash (`internal/agent`, `internal/slash`)

- [x] `aimscmd.go`: `/aims` (all areas: rating, 7d, streak, last note),
  `/aims <area>` (history with ids, 14 days), `/aims rubric` (the anchor
  table as text), `/aims block <area> <from> <to>`.
- [x] `slash.Catalog()` gains `aims` with `Args: true`. Pendant `cmds`
  frame and Telegram menu follow for free.
- [x] Tests: `aimscmd_test.go` parse, `aims_handle_test.go` each form,
  `/help` contains `/aims`.

### Prompt (`internal/cron`, `internal/agent`, `internal/memory`, persona)

- [x] `cron/planner.go` `DefaultDailyPlannerPrompt`: the four lines in
  §6. `plannerToolFirstNote` gets the one-sentence version.
- [x] `aims/rubric.go`: the rubric and anchor table as one constant;
  used by the planner prompt, `/aims rubric`, and the `aim_log`
  description.
- [x] `memory/tools.go` `memory_store` description: one clause — the
  sentence stays here, what happened goes to `aim_log`.
- [x] `examples/persona/PERSONA.example.md` (and the native / docker /
  hosting copies): one chat shot — "I forgot, I ran Tuesday" and "that
  dinner was planned". Keep the file under budget; trade a line if
  needed.
- [x] Tests: `cron/planner_test.go` needles (`aim_log`, `aim_history`,
  `ladder` words), `agent/harness_test.go` planner note needles.

### Wiring (`cmd/gantry/run.go`)

- [x] Open `aims.Store` on the shared DB after `cron`; add
  `aims.Composite` to the tool stack; pass the store to
  `agent.Options.Aims` for stamps and `/aims`.
- [x] `memory_forget` on `aim/<area>` calls `aims.Forget`.
- [x] `gantry status` untouched. Log one `aims ready` line at boot with
  the live area count.

### Eval (`internal/agent/testdata/eval`, `eval_harness_test.go`)

- [x] Harness: fixture field `ledger` (events with scores) seeded
  before the turn; `aims.Composite` in the canned stack; outcome
  collects `aim_log` calls and the post-turn ledger.
- [x] Gate `evidence_from_turn`: every `aim_log` `what` shares a token
  run with a tool result or the inbound text.
- [x] `07_planner_gym_no_workout`: add `ledger` with yesterday `-1`
  note `nudged`; expect `aim_log` from the tool and the ask rung with
  `[wait]`; forbid the nudge wording.
- [x] New `16_planner_streak_credit`.
- [x] New `17_planner_weight_on_pace`.
- [x] New `18_chat_night_out` (three areas, one `aim_log`).
- [x] New `19_chat_rescore` (`event=` with `weight:0`).
- [x] `10_planner_weight_dinner`: expect `aim_log`.
- [x] `TestEvalFixtures_WellFormed` covers the new field. Run
  `make integration-test EVAL_ARGS='-eval.n=3 -eval.only=…'` on the
  six; record rounds and tokens in this doc.

Live eval 2026-09-25, `gemini-3.6-flash`, n=3, 18/18 passed. A round
over the fixture budget is reported, not failed.

| Fixture | Mean rounds | Prompt | Completion | Over budget |
| --- | --- | --- | --- | --- |
| `planner_gym_no_workout` | 3.33 | 27.2k | 235 | 1/3 |
| `planner_weight_dinner` | 3.00 | 24.0k | 210 | 3/3 |
| `planner_streak_credit` | 3.67 | 31.2k | 224 | 2/3 |
| `planner_weight_on_pace` | 3.33 | 27.2k | 213 | 1/3 |
| `chat_night_out` | 2.00 | 12.1k | 106 | 0/3 |
| `chat_rescore` | 3.00 | 17.4k | 101 | 3/3 |
| all six | 3.06 | 23.2k | 182 | 10/18 |

### Docs

- [x] `docs/cron.md`: a short "Aims ledger" section pointing here;
  `[progress]` in the planner section.
- [x] `docs/features.md`: one bullet under Time; a row in The Okay
  ("scores are the agent's; labeled").
- [x] `docs/persona_doc_goals.md`: scenario rows for the new fixtures.
- [x] `docs/architecture.md`: `[progress]` in the prompt assembly
  order.
- [x] `docs/gantree-contract.md`: **Phase 3**, not now. Leave a line
  that the tables exist and the schema is not yet stable.
- [x] `readme.md` and `docs/dockerhub.md`: `/aims` in the command row.

### Phase 1 – 3 (not yet)

- [ ] 30d / 90d slope and weekly buckets.
- [ ] Block %: planned = pinned cues fired, done = up-days.
- [ ] Pearson: score vs measurement; aim A day vs aim B next day; ≥ 8
  buckets or "too early".
- [ ] Gantree contract rows.
