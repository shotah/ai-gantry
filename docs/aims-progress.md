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
| 30d | mean day score (the rating); per (area, metric): latest, mean, slope | rating |
| weeks | Sunday-start buckets from the aim's first event, up to 13: mean score, up / against, mean of each metric; one slope over the week means | trend, read once a week |
| correlation | weekly mean score vs weekly mean measurement under one aim; and day score of aim A vs next-day score of aim B on days A was scored; Pearson at ≥ 8 buckets and `|r| ≥ 0.3`, absent before | effort vs outcome; aim vs aim |
| block | over `[from, to]`: up-days / days, mean score | 6-month adherence |
| last note | days since `nudged` / `asked` / `offered` / `praised` per aim | the groundhog guard |

Block bounds: `/aims block <area> <from> <to>`, or the agent sets it
when the sentence has a date ("by spring"). The kernel does not parse
the sentence. Planned-vs-done is not computed: a fired once-cue is
disabled and a repeating cue keeps one `last_run_at`, so `cron_job`
cannot say how many cues fired in a span, and a fire log is more
table than the number is worth.

### 4.4 Where it shows

| Surface | Content | Cost |
| --- | --- | --- |
| `[aims]` every turn | Suffix per aim: `training: gym 3 mornings/wk — 30d +1.4 · 7d +6 · streak 2 · asked 1d ago` | ~25 chars per aim. Cap stays at 5. Stamp, no tool call. |
| `[progress]` on the planner turn only | Per aim: rating, a **5-day grid by date** (day score and that day's events with ids; an empty day shows `·`), 14d / 30d, measurement trend, block %, last note. On the week's first planner (Sunday in `CRON_TZ`) the week lines and any correlation join it; the other six mornings do not carry them | Planner is the one burn; pay it there. The weekly review is the same turn, once a week. |
| `/aims` | Same block; `/aims <area>` full ledger with ids; `/aims rubric` the anchor table | Phase 0 |
| Pendant board | Same numbers as `/aims`, as a mailbox snapshot, optional screen | Push on dial and after a ledger write. Not a query. |
| Gantree | Reads `aim_event` / `aim_score` in `gantry.db` | Contract row once the schema settles |

The grid is by date, not by row count. Five days with nothing on
Wednesday shows Wednesday as `·`, which is the fact.

### 4.4.1 Pendant board

The phone does not open `gantry.db`. Pendant and Cab only see the
mailbox. Gantree is the process that can read the file, and it is the
operator's yard, not the human's pocket. So the board is a frame the
crane pushes, the same way `cmds` arrives on dial
(`internal/channel/pendant/inbound.go`). No new socket, no status
field, no HTTP route.

**The pendant side is built and waiting on this frame** (gantry-pendant
`docs/frontends.md` → Aims board: Worker stores the latest and
replays it on phone connect right after `cmds`; the PWA paints a Goals
drawer; every button there is a `/aims …` turn). Its parser,
`lib/mailbox/aims.ts`, is the contract. **These are the json tags.**
Anything else is dropped on the phone; a half-formed optional object
is dropped whole; a missing optional line is painted as nothing.

```json
{
  "kind": "aims",
  "aims": [
    {
      "area": "training",
      "sentence": "gym 3 mornings/wk",
      "rating30": 1.4,
      "sum7": 6,
      "streak": 2,
      "note": "asked",
      "note_at": "2026-09-25",
      "days": [
        { "day": "2026-09-22", "score": 2, "events": [411] },
        { "day": "2026-09-23", "score": -1, "events": [413] },
        { "day": "2026-09-24", "score": 0, "events": [] },
        { "day": "2026-09-25", "score": 3, "events": [415] },
        { "day": "2026-09-26", "score": 0, "events": [416] }
      ],
      "weeks": [
        { "start": "2026-09-13", "mean": 0.9, "up": 3, "against": 1, "metrics": [] },
        { "start": "2026-09-20", "mean": 1.4, "up": 4, "against": 1,
          "metrics": [{ "metric": "weight", "mean": 191.4, "unit": "lb", "n": 3 }] }
      ],
      "slope": 0.3,
      "block": { "days": 10, "up": 4, "against": 2, "mean": 0.4, "pct": 0.4 },
      "effect": { "a": "training", "b": "", "metric": "weight", "r": -0.42, "n": 9 }
    }
  ],
  "links": [
    { "a": "training", "b": "weight", "r": 0.38, "n": 12 }
  ]
}
```

| Field | From | Rule |
| --- | --- | --- |
| `aims[]` | `capAreas` order | Live `aim/<area>` rows, `bootstrap` skipped, **cap 5**. `[]` is a real frame: the screen clears. |
| `area` | the `<area>` key | `[a-z0-9][a-z0-9_-]*`, lowercase. |
| `sentence` | the memory row | ≤ 240 chars. Required; a row without one is dropped. |
| `rating30`, `sum7`, `streak` | `Stats.Rating30`, `Stats.Sum7`, `Stats.Streak` | Always present. `0` when `!HasEvents`. |
| `note`, `note_at` | `Stats.LastNote`, `Stats.LastNoteAt` | `note` always present (`""` when none). `note_at` local `YYYY-MM-DD`, `omitempty`. The phone shows the word, not the age. |
| `days[]` | `DayScores(today-4, today)` | Oldest first, **always five**; an empty day is `{ "score": 0, "events": [] }`. `events` are `Event.ID`s — the same ids `/aims <area>` prints, so the human can quote one. Phone accepts up to 14. |
| `weeks[]` | `Weeks(ctx, area, now)` | Oldest first, **cap 13**, `omitempty` when none. `start` is the Sunday; `mean`, `up`, `against` as computed; `metrics[]` is the `Metrics` map flattened, one entry per (metric, unit): `{ metric, mean, unit, n }`, `[]` when none. |
| `slope` | `WeekSlope(weeks)` | Per week. Omit when `!ok`. |
| `block` | `BlockStatsAt` | `{ days, up, against, mean, pct }`. Omit when `!ok` or `Days == 0`. |
| `effect` | `Effect(area, weeks)` | The `Corr` as is. Omit unless `StampCorr(ok, r)`. |
| `links[]` | `NextDay` over `capAreas` pairs | Same set `crossLines` prints: strongest `\|r\|` first, **cap 3**, only `StampCorr` hits. `{ a, b, r, n }` (`Corr.Metric` is empty here; the phone ignores it). `omitempty`. |
| `user_id` | — | Omit. Room-wide, like `cmds`. (The Worker also accepts a per-human board under `user_id`; not needed for one human.) |

Send it on dial, after `cmds`, and again after any `runTurn` or cron
`Push` where the rendered JSON differs from the last one sent on this
connection — that covers `aim_log`, an aim `memory_store`, a forget
cascade, and `/aims block`. Not on an ordinary chat turn that changed
nothing. A reconnect resets the memory, so every dial gets a board.
The phone never asks for it; the way back in is `/aims`, `/aims
<area>`, `/aims rubric` as ordinary inbound turns from the drawer's
buttons. Telegram has no board; `/aims` remains its text view.

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
| **1. Read it back** | week buckets and one slope; block %; Pearson or nothing; the week's first planner reads them back; gantree contract rows for the three tables | "how am I doing" and the correlation ask |
| **2. Board** | the `kind: "aims"` frame in §4.4.1 from the numbers Phase 1 already computes; sent on dial and when it changes. No new tool, no model call, no HTTP | the phone opening `/aims` as text to see a number |

Phase 0 is shipped (live eval 2026-09-25, 18/18). Phase 1 is shipped.
Phase 2 is the [Pendant frame](#pendant-frame) list in §10; the
receiving side (Worker + PWA) is already live in gantry-pendant.

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
- `planner_week_start`: Sunday, nine weeks, weight falling. The reply
  names training and weight and a direction. Not a made-up correlation.
- `planner_midweek_quiet`: the same ledger on Wednesday, yesterday
  `+2`, calendar already a work day. `[silent]`. No week summary.

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

Live eval 2026-09-26, `gemini-3.6-flash`, n=1, 8/8 passed. Same
six, plus the week-start and midweek fixtures. A round over budget
is reported, not failed.

| Fixture | Rounds | Prompt | Completion | Over budget |
| --- | --- | --- | --- | --- |
| `planner_gym_no_workout` | 3 | 24.1k | 178 | 0 |
| `planner_weight_dinner` | 3 | 24.4k | 222 | 1 |
| `planner_streak_credit` | 3 | 24.1k | 148 | 0 |
| `planner_weight_on_pace` | 3 | 23.2k | 134 | 0 |
| `chat_night_out` | 2 | 11.7k | 114 | 0 |
| `chat_rescore` | 3 | 18.0k | 111 | 1 |
| `planner_week_start` | 4 | 34.9k | 399 | 1 |
| `planner_midweek_quiet` | 2 | 14.8k | 126 | 0 |
| all eight | 2.88 | 21.9k | 179 | 3/8 |

### Docs

- [x] `docs/cron.md`: a short "Aims ledger" section pointing here;
  `[progress]` in the planner section.
- [x] `docs/features.md`: one bullet under Time; a row in The Okay
  ("scores are the agent's; labeled").
- [x] `docs/persona_doc_goals.md`: scenario rows for the new fixtures.
- [x] `docs/architecture.md`: `[progress]` in the prompt assembly
  order.
- [x] `docs/gantree-contract.md`: leave a line that the tables exist
  and the schema is not yet stable. The real column list is Phase 1.
- [x] `readme.md` and `docs/dockerhub.md`: `/aims` in the command row.

### Phase 1

One phase. Week buckets, block %, correlation, the weekly read-back,
and the yard contract ship together. No new tool, no second model
call, no second cron job, no chart in chat, no mailbox frame.
`[aims]` on ordinary turns stays the Phase 0 suffix.

The weekly review is the daily planner. The harness adds the week
lines to `[progress]` only on the week's first planner run (Sunday in
`CRON_TZ`, the same `sundayStart` the `[time]` week grid uses). The
other six mornings carry the Phase 0 grid and nothing more, so a
smaller model pays for the long view once a week. `/aims` shows all
of it any day; that is the human's pull, not a model turn.

Every window starts at the aim's first event. Days before the aim
existed are not zeros; they are not in the fit.

A line is done when `go test ./...` and `golangci-lint run ./...`
pass. The prompt lines also pass the live eval (§7) at `-eval.n=1`.

#### Weeks (`internal/aims`)

`SeriesAt` already returns latest / mean / slope for one metric.
This adds the week buckets and one slope over them. No `Slope30`,
`Slope90`, or `Rating90`: the week means are the trend.

- [x] `weeks.go`: `Week{Start string; Mean float64; Up, Against int;
  Metrics map[string]Measure}` where `Measure{Mean float64; Unit
  string; N int}`. `Weeks(ctx, area, now) []Week`: Sunday-start local
  weeks from the week of the first live event through the week that
  contains `now`, cap 13 (oldest dropped). Mean includes zero days
  inside the aim's life. Metric mean is over that week's rows with
  the unit as logged; mixed units stay as logged, no conversion.
- [x] `WeekSlope(weeks []Week) (float64, bool)`: OLS of `Mean` per
  week index, `ok` false under 2 weeks. Reuse the fit in
  `slopePerDay`; do not write a second one.
- [x] Tests in `math_test.go`: the aim's first event on a Wednesday
  gives a first bucket of 4 days, not 7 zeros. A flat +1 gives slope
  `0`. Four weeks of `0` then four of `+2` give a positive slope. A
  week that crosses a month boundary in `America/Los_Angeles` is one
  bucket. An empty week inside the aim's life is `Mean 0`, not
  omitted. Weights in `lb` and `kg` in one week are two entries in
  `Metrics` by unit, not one number.

#### Block (`internal/aims`)

`aim_block` and `/aims block` already store `[from, to]`. This
computes the line. No block row means no line. Planned-vs-done is
not computed (§4.3); `cron_job` does not keep the fires.

- [x] `block.go`: `BlockStats{Days, Up, Against int; Mean, Pct
  float64}`. `Days` is inclusive of both ends, clipped to today.
  `Up` is days with score `> 0`. `Mean` includes zeros. `Pct` is
  `Up / Days`, `0` when `Days == 0`. `BlockStatsAt(ctx, area, now)
  (BlockStats, bool)`; `false` when no row.
- [x] Tests: no block row is `false`. Four up-days of ten is `0.4`. A
  span that ends next month is clipped to today. Month boundary in
  `America/Los_Angeles`.

#### Correlation (`internal/aims`)

Pearson on the week buckets and on the next-day join. Under the gate
the result is absent: no number, no "too early" text in the stamp.
The prompt tells the model a missing line means nothing is known.

- [x] `corr.go`: `Pearson(xs, ys []float64) (r float64, ok bool)`.
  `ok` is false when `len(xs) < 8`, lengths differ, or either side
  has zero variance.
- [x] `Effect(area string, weeks []Week) (Corr, bool)`: the aim's
  latest metric (same pick as `[progress]`), weekly `Mean` score
  against weekly metric mean, one unit. `n` is weeks where both
  exist. `Corr{A, B, Metric string; R float64; N int}`.
- [x] `NextDay(ctx, a, b string, now) (Corr, bool)`: day score of
  `a` on days `a` has a live event, against day score of `b` on
  `day + 1` (missing `b` day is `0`). 90 days ending today. `n` is
  the count of those `a` days. Zeros-only `a` days do not count.
- [x] Both are stamped only when `ok` and `|r| ≥ 0.3`. Twenty ordered
  pairs among five aims will produce a small `r` by chance; the floor
  is the guard, not a p-value.
- [x] Tests: the §4.2 week is `ok == false` on both. Eight weeks
  where the metric falls as the score rises gives `r < 0`. A next-day
  join with 89 days of zeros and 3 scored days is `n = 3`, not 89.
  Identical inputs give `r = 1`. A 7-week pair is `ok == false`.
  `|r| = 0.2` with `n = 12` is not stamped.

#### Stamps and `/aims`

- [x] `Store.ProgressText(ctx, areas, now)` grows a week-start flag:
  `now` is the first day of its Sunday week → each aim adds, under
  its five-day grid, `weeks: +0.3 +0.5 +0.1 …` (oldest first, cap 8
  in the stamp), `slope ±x/wk`, the block `up/days (pct)` when set,
  and one `effect r=±x.xx (n)` when it passes. Cross-aim
  `drinking → next-day climbing r=+0.42 (12)` lines sit once under
  all aims, strongest `|r|` first, at most 3.
- [x] Any other day: the Phase 0 stamp exactly. The block line is the
  one exception; it is short and it is daily adherence.
- [x] `[aims]` on ordinary turns does not change.
- [x] `/aims` shows the block line and the cross-aim lines every day.
  `/aims <area>` shows the last 8 weeks, the slope, and the effect
  line, every day.
- [x] Tests: `prompt_payload_test.go` chat turn still has no
  `[progress]`. A planner turn on Sunday `2026-10-04` with 9 weeks of
  fixture data contains `weeks:`, `slope`, and one `r=`. The same
  fixture on Wednesday `2026-10-07` contains the five-day grid and
  the block line and none of `weeks:`, `slope`, `r=`. A 7-week
  fixture on a Sunday contains `weeks:` and no `r=` and not the words
  "too early". `/aims <area>` on a Wednesday contains `weeks:`.

#### Prompt

The planner already has the ladder. This adds how to read the week
lines and forbids inventing a trend when they are absent.

- [x] `DefaultDailyPlannerPrompt`, one paragraph before the ladder:
  when `[progress]` carries `weeks:` it is the week's first session.
  One line per aim from those numbers (the run of week means, the
  slope, the block fraction, the effect line if present) before the
  ladder, then the ladder as usual; that reply is not `[silent]`. No
  `weeks:` line means an ordinary morning; do not summarize the week.
  A missing `r=` is not "no correlation" and not a guess; say nothing
  about it. On-pace measurement stays `[silent]`.
- [x] `plannerToolFirstNote`, one sentence: `weeks:` in `[progress]`
  means read the week back first, one line per aim, then the ladder.
- [x] `cron/planner_test.go` needles: `weeks:`, `slope`, `r=`.
  Existing needles stay.
- [x] Fixture `20_planner_week_start.json`: Sunday, 9 weeks of ledger
  on training and weight with the weight metric falling, expect a
  reply that names both aims and a direction word (`up|down|flat|
  steady|slope|trend`), `silent: false`, `reply_not` "too early" and
  `correlat`, `round_budget 3`. Fixture `21_planner_midweek_quiet.json`:
  same ledger on the Wednesday, yesterday `+2`, expect `silent: true`
  and `reply_not` `week`.
- [x] Live eval at `-eval.n=1` over all eight fixtures. Record rounds
  in §7. A ladder or week-start miss is a prompt fix, not a looser
  expect. 2026-09-26: 8/8. The week read-back sits at the end of
  `plannerToolFirstNote`; the midweek fixture's calendar is a work
  day, so the empty-calendar ask does not fire.

#### Pendant frame

Phase 2. The wire is fixed in §4.4.1 — the phone already parses it.
This repo renders and sends; the receiving Worker and PWA are shipped
in gantry-pendant (`docs/frontends.md` → Aims board). Cab may ignore
the kind. Telegram has no board.

There is no hook from `aim_log` or `memory_store` to the mouth, and
none is added. The channel renders the board itself and sends it when
the JSON changed. That covers `aim_log`, an aim `memory_store`, a
forget cascade, and `/aims block` with one comparison.

- [x] `board.go` in `internal/aims`: `Row` and `Board(ctx, areas,
  now) ([]Row, []Link, error)`. `Row{Area, Sentence string; Rating30
  float64; Sum7, Streak int; Note string; NoteAt string; Days
  []DayCell; Weeks []WeekCell; Slope *float64; Block *BlockStats;
  Effect *Corr}`, `DayCell{Day string; Score int; Events []int64}`,
  `WeekCell{Start string; Mean float64; Up, Against int; Metrics
  []Measure}`, `Link{A, B string; R float64; N int}`. json tags
  **exactly** the §4.4.1 names, lowercase, `omitempty` on `note_at`,
  `weeks`, `slope`, `block`, `effect`; `days` and `metrics` never
  omitted (`[]`). `Measure` gains tags `metric`, `mean`, `unit`, `n`.
  `BlockStats` gains `days`, `up`, `against`, `mean`, `pct`; `Corr`
  gains `a`, `b`, `metric`, `r`, `n`. Same gates as the stamps:
  `Slope` nil when `WeekSlope` is `!ok`; `Block` nil when `!ok` or
  `Days == 0`; `Effect` nil unless `StampCorr`; `Links` is what
  `crossLines` prints, cap 3, strongest first. Reuse `capAreas`,
  `StatsAt`, `DayScores`, `Weeks`, `Effect`, `NextDay`,
  `BlockStatsAt`. Do not write a second computation of any of them.
- [x] `Channel` option `Board func(ctx) ([]aims.Row, []aims.Link,
  error)`. Nil means no `aims` frame ever (Telegram-only installs,
  tests that do not care). `cmd/gantry/run.go` wires it from the
  store with the live `Areas`.
- [x] `outboundFrame` gains `Aims []aims.Row` tagged `aims` and
  `Links []aims.Link` tagged `links,omitempty`. `aimsFrame(rows,
  links)` builds `{"kind":"aims","aims":[…],"links":[…]}`. Empty
  rows is `"aims":[]`, still sent, so a screen can clear. No
  `user_id`.
- [x] Send on dial after `cmdsFrame()` and before `allowFrame`. After
  every `runTurn` and every cron `Push`, render again and `writeOn`
  only when the marshalled JSON differs from the last one sent on this
  `conn`. A reconnect resets that memory.
- [x] Tests in `pendant_test.go`: dial writes `cmds`, `aims`, `allow`
  in that order. The `aims` body round-trips through the §4.4.1
  example (golden file). A turn that does not change the board writes
  no `aims` frame. A turn whose handler logs an event writes one. A
  handler that forgets `aim/training` writes a board without that
  row. Nil `Board` writes no `aims` frame and no error. One week
  still sends `weeks` (the table omits that key only when there are
  none) and omits `slope` and `links`; nine correlated weeks send
  both `weeks` and `links`.
- [x] `docs/channels.md`: one line that dial sends `aims` after `cmds`
  and a changed board resends it. No new socket, no `gantry status`
  field, no HTTP.
- [x] `docs/features.md`: the Time bullet gains "and a goals board on
  the phone".

#### Yard contract (`docs/gantree-contract.md`)

Read-only. `gantry status` JSON stays as it is. Gantree opens
`gantry.db`; it does not get a hook inside the process. Do not edit
the gantree checkout; the contract page is the handoff.

- [x] Replace the "schema is not stable" stub with the three tables:
  `aim_event`, `aim_score`, `aim_block`. Columns as in §4.7. Say
  `day` is a local `YYYY-MM-DD` in `CRON_TZ`, live rows are
  `superseded_by IS NULL`, `score` is the agent's opinion `-3…+3`
  and not a grade, and the day score is `SUM(score)` over live rows
  per (area, day) clamped to `[-3, 3]`.
- [x] Say what not to chart: superseded rows, `aim/bootstrap`, and a
  correlation the crane did not stamp. Weeks, slope, and Pearson are
  derived; the yard may recompute them, it should not add a table.
- [x] `docs/todo.md` "Aims ledger" paragraph: Phase 1 shipped, link
  stays.

#### Docs

- [x] `docs/cron.md` planner section: the week's first session reads
  the week back; no second job.
- [x] `docs/features.md`: the Time bullet gains "weekly read-back on
  the planner turn"; The Okay row stays.
- [x] `docs/architecture.md`: `[progress]` note says week lines on
  the week-start planner only.
- [x] This doc: §7 eval table rows for fixtures 20 and 21; §5 stays
  one row.
