# Tasks (plan)

Status: **built**. Phase 0 and Phase 1 are in; the live eval row is in [§7](#7-tests-and-evals). The work list is [§9](#9-todo).

A pocket list of the things the human has to do, on `[harness]` every
turn, on the phone as a board, and gone when done. Built the way
`[aims]` was: one memory row per item, no new table, no new tool.

Sibling: [aims-progress.md](aims-progress.md). Aims are the months
sentence and its ledger. Tasks are this week's crap.

---

## 1. The problem

The agent already knows three kinds of open thing:

| Prefix | Who holds the ball | Stamp |
| --- | --- | --- |
| `aim/<area>` | the human, for months | `[aims]` |
| `waiting/<slug>` | someone else | `[loops]` |
| `follow/<slug>` | the agent | `[loops]` |

There is no row for *the human, this week*. "Call the dentist", "return
the Amazon box", "renew the passport" have nowhere to live, so they land
as a chat line the agent forgets by tomorrow, or as a `follow/` row that
means the wrong thing (the agent is not going to call the dentist).

The daily planner pulls calendar and mail and sees appointments, not
errands. The Goals drawer on the phone shows the aims and nothing
smaller.

---

## 2. The idea

**A task is one memory row the agent keeps.** `fact` + `todo/<slug>` +
the words. The agent adds it when the human says it, rewrites it when
the words change, and forgets it when it is done. That is the whole
store.

```text
[todo] #420 amazon: return the box (9d ago) · #412 dentist: call to book a cleaning (3d ago) · #418 passport: renew, Wed 11am
```

The kernel stamps the line every turn, oldest first, cap 5, `+N more —
/todo`. The agent reads it the way it reads `[aims]`: no recall, no tool
call, already there. Nothing on it expires. A task that has sat nine
days is still a task; the age is there so the agent knows how long it
has been nagging, not so the kernel can drop it.

The phone gets the same list as a `todo` frame on the mailbox and paints
a Tasks drawer the way the Goals drawer works: the board is pushed, and
every button is a `/todo …` turn. `/todo` on Telegram and stdio is the
text view.

Why memory and not a table: aims needed a ledger because progress is a
series. A task has no series; it is open and then it is gone. Memory
already has ids, `updated_at`, prefix listing, hydration exclusion,
`memory_store` and `memory_forget` in the catalog, and the yard can
already read the `memory` table. Adding a `task` table would add three
tools the model has to choose between and a second delete path.

Why one line and not a project: if the list needs projects, subtasks,
or assignees, it is the wrong tool. This is the list you would write on
the back of your hand — and on the back of your hand you underline the
one that matters. Priority is a `!` or `!!` leading the words, nothing
more ([§3](#3-vocabulary)). The kernel caps what it shows and says so
when the list is long.

---

## 3. Vocabulary

| Term | Meaning | Example |
| --- | --- | --- |
| **Task** | One `fact` row, subject `todo/<slug>`, content is the action in the human's words. Optional "by <when>" stays in the words. | `todo/dentist`: call to book a cleaning |
| **Slug** | The key after `todo/`. `[a-z0-9][a-z0-9_-]*`, same shape as an aim area. The agent picks it: the noun. | `dentist`, `passport`, `amazon` |
| **Id** | The memory row id. On the stamp, on `/todo`, on the phone. What `/todo done` and `memory_forget` take. | `#412` |
| **Age** | From `updated_at`, after the first day. Same `channel.Age` as `[loops]`. Never a reason to drop the row. | `(9d ago)` |
| **Update** | Same subject, new words. Memory supersedes the old row; the id changes; the age restarts. | `todo/passport`: renew, moved to Thu |
| **Priority** | A marker leading the words: `!!` urgent, `!` high, nothing normal. Three or more `!` read as `!!`. The agent's — the human's word or its own read of the stakes; the prompt says the convention and nothing more. Changed by the same-subject rewrite, or by the kernel from `/todo prio`. Every view sorts urgent, high, normal, then oldest first inside a level. | `todo/taxes`: `!! file the extension` |
| **Done** | The row is deleted. No done ledger. | `memory_forget id=412` |

No due field. "by Oct 15" is in the content; the agent reads it against
the week grid already in `[harness]`. A reminder at a time is not a
task field either: `cron_schedule when=… memory_subject=todo/<slug>`
pins the row into the wake, and that tool exists today. A wake whose
row was forgotten first says nothing about it (`jobMemoryBlock` runs
on the prompt alone) — that is the right behaviour for a done task.

---

## 4. Decisions

### 4.1 Who writes it

The agent. Add, update, and remove are `memory_store` and
`memory_forget` on `todo/<slug>`, tools it already has. The kernel
writes exactly once: the phone's checkbox.

| Who | How | Cost |
| --- | --- | --- |
| The agent, from chat | "I need to call the dentist this week" → `memory_store fact todo/dentist "call to book a cleaning this week"`. "Make that Thursday" → same subject, new words. "That's urgent" → same subject, `!! ` in front of the same words. "Booked it" → `memory_forget id=412` (the id is on the stamp). | one turn, no new tool |
| The agent, from a wake | A task's pinned cron fires; the agent nags, or asks, or forgets it if the human already said it was done in the meantime. | the wake it already scheduled |
| The phone, checkbox | `/todo done <id>` → kernel `Forget`. The human did the thing; that does not need a model. | no model burn |
| The phone, priority button | `/todo prio <id> !!` (or `!`, or nothing to clear) → kernel re-`Store` on the same subject with the marker leading the same words. Same shape as any update: new id, age restarts. | no model burn |
| The phone, add field | Plain text to the agent. The pendant sends `add to my list: <words>`; the crane assumes no prefix and no fixture checks for one (`22b_chat_todo_add_phone` runs that exact wording, 3/3). The agent names the slug and keeps the words. | one turn |
| The agent, on its own | Only when the human said it. `Never auto-save guesses` already applies. The planner does not invent errands from mail. | — |

No `/todo add`. A kernel slug from the first three words
(`call-the-dentist`) sits badly next to the agent's `dentist`, and the
agent is the one that has to read the list back for weeks. One turn per
add is the price of a list the agent owns.

### 4.2 The key

`todo/<slug>`. Same kind+subject replaces the live row (memory's rule),
so "actually, book it for the 12th" is a re-store on `todo/dentist`,
not a second task. The old row is superseded; the id changes; the
phone repaints from the next board push, which follows the same turn.

### 4.3 Where it shows

| Surface | Content | Cost |
| --- | --- | --- |
| `[todo]` every turn | `#id slug: words (age)`, urgent then high then normal, oldest `updated_at` first inside a level, cap 5, `(+N more — /todo)`. The marker stays in the words (`#430 taxes: !! file the extension`) so the agent sees it. No stale cue, no falloff. Absent when there are no rows. | one line; same budget rule as `[loops]` |
| `/todo` | Every open row with ids, same order as the stamp. When more than 10 are open the footer says `14 open — a pocket list; prune, or use a tracker`. `/todo done <id\|slug>` and `/todo prio <id\|slug> [!!\|!]` are the two writes. Mirrors `/aims`: a board, a kernel write or two, the rest is the agent. | kernel, no model |
| Pendant board | `todo` frame on the mailbox, room-wide, same dial-and-diff path as `aims` ([§4.4](#44-pendant-frame)). | rendered per turn, sent when changed |
| Telegram, stdio, Discord, Slack | `/todo` text. No board. | — |
| Gantree | Reads `memory` rows with `subject LIKE 'todo/%' AND superseded_by IS NULL`. No new table, no contract change beyond one sentence. | — |

Stamp order inside `[harness]`: `[aims]`, then `[todo]`, then
`[progress]`, then `[loops]`. Header word is `horizon`, same as the two
beside it, so the header does not grow.

Oldest first is deliberate. `[loops]` is newest first because a fresh
wait is the live one. A to-do list is the other way: the thing that has
sat nine days is the one to say out loud — unless something is marked
`!!`, which goes first however fresh it is.

### 4.4 Pendant frame

The phone does not open `gantry.db`. Same shape as the aims board
([aims-progress.md §4.4.1](aims-progress.md#441-pendant-board)): a
frame the crane pushes on dial and again when the JSON changed. No new
socket, no status field, no HTTP route. The receiving Worker and PWA
are gantry-pendant's; this repo renders and sends.

```json
{
  "kind": "todo",
  "todo": [
    { "id": 430, "slug": "taxes",    "text": "file the extension",       "at": "2026-10-09", "priority": 2 },
    { "id": 412, "slug": "dentist",  "text": "call to book a cleaning",  "at": "2026-09-23" },
    { "id": 418, "slug": "passport", "text": "renew, by Oct 15",         "at": "2026-09-26" }
  ]
}
```

| Field | From | Rule |
| --- | --- | --- |
| `todo[]` | `ListBySubjectPrefix(fact, todo/)` | Priority first, then oldest `updated_at` inside a level. **No cap** on the frame (the phone scrolls; the stamp is what is capped). `[]` is a real frame: the drawer clears. |
| `id` | `Entry.ID` | What the checkbox sends back as `/todo done <id>` and the priority button as `/todo prio <id> [!!\|!]`. An id the kernel no longer has (the agent rewrote the row between paint and tap) answers `todo: #418 is gone — the list was updated` and the next board fixes the drawer. |
| `slug` | subject after `todo/` | `[a-z0-9][a-z0-9_-]*`; a row that fails the pattern is dropped from the frame, not from memory. |
| `text` | `Entry.Content` minus the marker | Whitespace-collapsed, clipped to 240 runes. Empty is dropped (a row that is only `!!` is dropped). |
| `at` | `updated_at` (`created_at` fallback) | Local `YYYY-MM-DD` in the store zone. The phone shows the age; the crane does not compute it. |
| `priority` | the marker | `2` urgent, `1` high. **Omitted** when normal, so a list without markers is byte-for-byte the old frame. The phone paints a badge; the text has no `!` in it. |
| `user_id` | — | Omit. Room-wide, like `cmds` and `aims`. |

Dial order becomes `cmds`, `aims`, `todo`, `allow`. Send again after
every `runTurn` and cron `Push` when the marshalled JSON differs from
the last one on this conn; a reconnect resets that memory. Nil render
function means the frame is never sent.

Implementation: a second `Config` func beside `Board` and a second
sent-body string beside `aimsSent`. Two of a thing is not yet a list;
if a third board arrives, generalise then.

### 4.5 What the agent does with it

The agent keeps the list and keeps at it. The things on it are the
human's to do; the nagging is the agent's to do. A task the human has
not done is not a task that stopped needing doing.

| Turn | Rule |
| --- | --- |
| Ordinary chat | Bring a task up only when the conversation touches it ("I'm downtown" → "the dry cleaner is on your list"). Never recite the list. |
| The human states a task | Store it. One row, their words, a slug that is the noun. No confirmation paragraph; one short line or a reaction. |
| How much it matters | `!! ` or `! ` in front of the words, same subject — their word or the agent's read of the stakes. Their word beats the agent's read. The prompt does not list what counts as stakes; the eval ([§7](#7-tests-and-evals), 26/26b) shows the model gets a passport-before-a-flight right and leaves a no-rush garage alone without being told. |
| The words change | Same subject, new words. "Make that Thursday", "the other dentist", "actually two boxes" are rewrites, not new rows. |
| The human says it is done | `memory_forget` the id from the stamp. Not a query — a query on "dentist" deletes the dentist's phone number too. Only the human closes a task; the agent never decides one is done. |
| Planner turn | Read the words against the week grid. A task whose words name today ("Wed 11am", "by Friday" on Friday) gets `cron_schedule when=<that time or a sensible one> memory_subject=todo/<slug>` in the same tool batch — the wake carries the row. Then one line per task that is overdue by its own words, and one line for the oldest task past a week — every planner, until it is gone. `[todo]` arrives in priority order; the planner is told a marker is its to move when the week says so, and no more. Not `[silent]` when a line exists. The chat closer stays off this turn: it was not asked by them. |
| A day that is not the task's day | Nothing. Monday does not schedule Wednesday's cue; Wednesday's planner does. The words are the due date and the planner is the parser. |
| The nag | Never the same sentence twice; the age is in the stamp, so the line can move: day 2 names it, day 5 asks what is in the way, day 9 offers to put an hour on the calendar (ask first — that is a calendar write). None of those rungs is "shall I drop it". Dropping is the human's word, then `memory_forget`. |
| Long list | The `/todo` footer says it. The planner may say it once: this is a pocket list. Do not offer to reorganise it, and do not offer to prune it — the human prunes. |

No stale cue on the stamp, no ageing out, no "still on your list?"
rung. The aims ladder needed a note field because the ledger decides
silence; here the age is the only state and the answer to "how long has
this sat" is right there in the parentheses.

### 4.6 Storage

None new. The `memory` table as it is. The prefix is one constant next
to `SubjectWaitingPrefix`; `dropStamped` learns it so the rows are not
paid twice in `[memory]`.

---

## 5. Phases

| Phase | Ships | Not yet |
| --- | --- | --- |
| **0** | Prefix constant, `[todo]` stamp, `memory_store` description clause, `/todo` (list, done), planner sentence, tests, four eval fixtures. Works on every mouth. | — |
| **1** | `todo` frame on the pendant mailbox, `docs/channels.md` line, `docs/features.md` line. The drawer itself is gantry-pendant. | — |

There is no Phase 2. If a Phase 2 is wanted, re-read §2.

---

## 6. Prompt contract (Phase 0)

Four strings change. Each gets a needle test.

- `memory_store` description, as shipped: `A thing THEY have to do,
  even in passing (call, return, renew): fact subject=todo/<slug>
  this turn — not follow/. Store it and say you added it; never ask
  whether to add it. Doable now: say do it now, not good luck and not
  later. A future day named in the words: no cron_schedule for it and
  do not offer a reminder; that day's planner sets the cue. [todo] on
  [harness] is that whole list with #ids, so never memory_recall for
  it: same subject rewrites; done is memory_forget by the #id on
  [todo], only when they say so.` The `waiting/` and `follow/` clauses
  before it say whose ball it is (`Waiting on someone else`, `A note
  for you to follow up`). The first eval
  ([§7](#what-the-eval-taught)) is what put `todo/` next to `follow/`
  apart; the capture-now wording came after.
- `memory_forget` description gains: `A [todo] item is its #id on
  [harness]: forget that id, never a query (a query takes other rows
  with it), no memory_recall first.`
- `plannerToolFirstNote` gains `[todo]` in the "already in [harness]"
  list and one sentence at the end (end, not middle; the eval taught
  that): `A [todo] item whose words name today: cron_schedule its cue
  now with memory_subject=todo/<slug>, and the line says do it now —
  not good luck, not later. Each one overdue by its words, and the
  oldest past a week, is one line, a different line from yesterday,
  do it now — not the list, not [silent], never "shall I drop it". A
  turn was not asked by them: no closer, no "Anything else". [silent]
  stays [silent].`
- `DefaultDailyPlannerPrompt` gains the same rule as its last line.

Persona: `docs/persona.md` memory row gains `todo/` beside `waiting/`
`follow/`, and one sentence: the agent keeps the list; only the human
closes a task; forget by id.

---

## 7. Tests and evals

Unit, in the packages touched:

- `internal/memory/horizon_test.go`: `FormatTodo` orders oldest first,
  caps at five with `+N more — /todo`, shows `#id`, ages after a day,
  carries no cue at 30 days (a `[loops]` row the same age does), empty
  on no rows.
- `internal/agent/prompt_payload_test.go` (full board): `[todo]` sits
  after `[aims]` and before `[progress]`; the header still says
  `horizon` once; a `todo/` row is not in `[memory]`.
- `internal/agent/todocmd_test.go`: `/todo` lists with ids oldest
  first; `/todo done 412` and `/todo done dentist` forget the same
  row; a gone id answers with the "list was updated" line and no
  error; `/todo add` is usage, not a write; the long-list footer at 11.
- `internal/channel/pendant/pendant_test.go`: dial writes `cmds`,
  `aims`, `todo`, `allow`; golden round-trip of the §4.4 example; an
  unchanged turn writes no `todo`; a handler that forgets a row writes
  one without it; nil render writes nothing.

Eval (`internal/agent/testdata/eval`, `-eval.n=1`):

| # | Fixture | Turn | Expect |
| --- | --- | --- | --- |
| 22 | `chat_todo_add` | "I need to call the dentist this week" | `memory_store` with subject `todo/…`, kind `fact`; reply ≤ 2 lines; no `[wait]` |
| 22b | `chat_todo_add_phone` | `surface: android`, `input: typed`; "add to my list: return the Amazon box" (the pendant's add-field wording) | same as 22; the words stored are theirs, no prefix in the row |
| 23 | `chat_todo_done` | `[todo] #412 dentist: …` stamped; "booked the dentist" | `memory_forget` with `id` 412, not `query`; no `memory_recall` |
| 24 | `planner_todo_stale` | planner, one task at 9 days, calendar a work day | reply names the slug once; `silent: false`; `reply_not` the other tasks' slugs and `(?i)drop|remove|forget|still (on|need)` |
| 24b | `chat_todo_update` | `[todo] #412 dentist: call to book a cleaning` stamped; "make the dentist thing Thursday" | `memory_store` on subject `todo/dentist` with `thursday` in content; no `memory_forget`; no second `todo/` subject |
| 25 | `planner_todo_today` | planner on a Wednesday (`now` frozen), `todo/passport: renew, Wed 11am` stored Monday | `cron_schedule` with `memory_subject` `todo/passport` and `when` `11:00`; no cron for the other task whose words say Friday |
| 26 | `chat_todo_urgent` | "I fly out Monday and my passport is expired. I'm booking the emergency renewal appointment first thing tomorrow, just put it on my list" | one `memory_store` on `todo/…` whose `content` starts with `!` — the agent's own read; nothing in the turn says urgent. No `[wait]` |
| 26b | `chat_todo_whenever` | "at some point I should clean out the garage, no rush" | one `memory_store` on `todo/…` whose `content` starts with a non-`!` character; no `cron_schedule` |

A miss on 23 (forget by query) or 24 (offering to drop it) is a
description or note fix, not a looser expect.

### What the eval taught

Live, `gemini-3.6-flash`, 2026-09-26. The first pass at n=1 had all
four planners green and all three chat turns red, for two reasons
that were both in the `memory_store` description:

- `todo/` sat right after `follow/` with nothing to tell them apart.
  The model filed "call the dentist" as `follow/dentist` and scheduled
  its own cron. Fix: "A thing THEY have to do (call, return, renew) …
  not follow/, and no cron_schedule for it; the day's planner sets
  the cue".
- Nothing said the `#id` on `[todo]` makes a lookup unnecessary. Done
  and update both opened with `memory_recall dentist`. Fix: "[todo] on
  [harness] is that whole list with #ids, so never memory_recall for
  it" on `memory_store`, and "forget that id, never a query, no
  memory_recall first" on `memory_forget`.

Second pass: add still asked "ping you at a specific time, or leave
it for the planner?" — the model turned "the planner sets the cue"
into an offer. Fix: "do not offer a reminder or ask when … one short
line back, no question". Third pass, all three green in two rounds
with one tool call each.

One expect moved: 23 had `\?` in `reply_not_regex`, which failed a run
that closed the task correctly and then offered a reminder for the
appointment they had just booked. That is not the contract in the
table above, so `\?` came off 23. It stays on 22 and 24b.

Fixtures 23 and 24b also gained an `aim/training` row (22 already had
one) so an empty `[aims]` does not pull the bootstrap question into a
turn about tasks.

Live eval 2026-09-26, `gemini-3.6-flash`, n=3, 23/24 runs passed. A
round over budget is reported, not failed. `chat_todo_add_phone` was
added after the rest, when the pendant asked whether its add-field
wording mattered; it ran alone at n=3.

| Fixture | Mean rounds | Prompt | Completion | Over budget | Passed |
| --- | --- | --- | --- | --- | --- |
| `chat_todo_add` | 2.00 | 11.8k | 48 | 0/3 | 3/3 |
| `chat_todo_add_phone` | 2.00 | 11.9k | 44 | 0/3 | 3/3 |
| `chat_todo_done` | 2.67 | 17.9k | 163 | 2/3 | 3/3 |
| `chat_todo_update` | 2.00 | 11.7k | 44 | 0/3 | 3/3 |
| `planner_todo_stale` | 2.33 | 18.7k | 179 | 0/3 | 3/3 |
| `planner_todo_today` | 3.00 | 24.7k | 203 | 0/3 | 3/3 |
| `planner_week_start` | 3.67 | 32.4k | 374 | 2/3 | 2/3 |
| `planner_midweek_quiet` | 2.00 | 15.3k | 126 | 0/3 | 3/3 |

### Priority (2026-10-09)

The first prompt draft listed what counts as stakes (a day within the
week, money, health, legal, travel) and told the planner to bump an
unmarked task to `!` two days out. It was cut to one sentence — `!!
urgent, ! high, none normal, theirs or your read of the stakes` — before
the first run, on the view that the model will work out what matters on
its own. It did, on both models:

| Fixture | Model | n | Mean rounds | Prompt | Passed |
| --- | --- | --- | --- | --- | --- |
| `chat_todo_urgent` | 3.6-flash | 3 | 2.00 | 13.8k | 3/3 — every run stored `!! …`; run 1 also set a cron for the appointment |
| `chat_todo_whenever` | 3.6-flash | 3 | 2.00 | 12.8k | 3/3 — plain words every run |
| `chat_todo_urgent` | **3.8-flash** | 3 | 2.00 | 14.4k | 3/3 — `!! …` every run, no `[wait]`; two runs also wrote a `self_note` |
| `chat_todo_whenever` | **3.8-flash** | 3 | 2.00 | 13.3k | 3/3 — plain words every run |
| 22, 22b, 23, 24, 24b, 25 | 3.6-flash | 1 | 2.33 | 17.8k | 6/6 — unmoved by the shorter clause |
| 22, 22b, 23, 24, 24b, 25 | **3.8-flash** | 1 | 2.17 | 18.1k | 6/6 |

The 3.6 run of 26 used a shorter inbound ("…I have to get the emergency
renewal appointment booked") and the first run, having stored
`!! Book emergency passport renewal appointment … for Monday Oct 12
flight` (right), asked whether the flight confirmation and photo were
ready and set `[wait]`. Rather than drop the no-question gate, the
inbound was rephrased to say they are already on it ("I'm booking … first
thing tomorrow, just put it on my list"), which closes the conversation
the way a real message would; `wait: false` stayed, and 3.8 passed it
3/3. The live model is `gemini-3.8-flash` from here.

`planner_week_start` is the aims fixture, run here to check the
`[todo]` planner sentence did not move it. Its one miss was no
`aim_log` on a turn with no `todo/` rows seeded (so no `[todo]`
stamp); it is not attributable to this change from one run and the
fixture was not touched. `chat_todo_done` runs long because the model
also files the booked appointment as `event/dentist` and enables the
calendar — reasonable, and reported, not failed.

---

## 8. Not this

- No `task` table, no `task_*` tools. Memory rows and the two tools the
  model already has.
- No due field, no due-date parser. The words carry "by Friday"; the
  week grid is already stamped.
- No project, subtask, tag, assignee, or recurrence. A recurring chore
  is `cron_schedule repeat=daily`, which exists. Priority is the one
  exception, and it is two characters in the words, not a field.
- No done ledger, no "you finished 4 this week". Forget deletes; that
  is the point of a pocket list.
- No auto-capture from mail or calendar. The human says it, or it is
  not a task.
- No manual reordering. Priority, then oldest first, everywhere.
- No ageing out, no stale cue, no "still on your list?" The kernel
  never drops a row and the agent never suggests it. Done is the
  human's word.
- No `/todo add`. The agent names and keeps the rows; the kernel's
  writes are the checkbox and the priority button, both of which only
  touch a row that already exists.
- No second model call. The stamp is a query and a format.

---

## 9. Todo

Phase 0 in build order. A line is done when `go test ./...`,
`golangci-lint run ./...`, and (for prompt lines) the named eval pass.

### Memory (`internal/memory`)

- [x] `horizon.go`: `SubjectTodoPrefix = "todo/"`. `FormatTodo(entries
  []Entry, now time.Time) string` — sort by `updated_at` ascending
  (`created_at` fallback), `horizonParts` with prefix `todo/` and
  `staleAfter` 0 (no cue), each part prefixed `#<id> `, more-hint
  `/todo`. `horizonParts` needs an id-prefix switch; `[aims]` and
  `[loops]` keep their output byte for byte.
- [x] `tools.go` `memory_store` description: the §6 clause. Needle
  test in `tools_extra_test.go`.

### Stamp (`internal/agent`)

- [x] `harness.go`: `horizon` gains `todo []memory.Entry`;
  `loadHorizon` lists `fact` + `todo/`; `dropStamped` includes it;
  `stamp` emits `[todo]` after `[aims]`, before `[progress]`;
  `harnessTags` gains `{"[todo]", "horizon"}` after `[aims]`.
- [x] `prompt_payload_test.go` full-board case grows a `todo/` row.
  Update `testdata/pendant/completer_fullboard_harness.txt`.

### Slash (`internal/agent`, `internal/slash`)

- [x] `slash.go` catalog: `{Name: "todo", Hint: "pocket list (done
  <id|slug>)", Args: true}`. Telegram menu, `/help`, stdio ready line,
  and the pendant `cmds` frame follow.
- [x] `todocmd.go`: `parseTodoCommand`, `handleTodo`, shaped like
  `aimscmd.go`. `/todo` list; `/todo done <id|slug>` → `Forget`
  (digits are an id; otherwise `ActiveByKindSubject(fact,
  todo/<slug>)`); a missing id or slug is the "list was updated" line,
  not an error. Anything else is `usage: /todo [done <id|slug>]`.
  Footer past 10 open. `todo: not configured` when memory is off.
- [x] Wire beside `parseAimsCommand` in `agent.go`.

### Prompt (`internal/agent`, `internal/cron`, persona)

- [x] `plannerToolFirstNote`: `[todo]` in the already-in-harness list;
  §6 sentence as the last sentence.
- [x] `DefaultDailyPlannerPrompt`: same rule as the last line. Needles
  `[todo]` in `planner_test.go`.
- [x] `docs/persona.md` memory row and one sentence.

### Eval

- [x] Fixtures 22–25 as in §7 (five files; 24b is
  `24b_chat_todo_update.json`). `evidence_from_turn` only where a
  tool call is expected. Run `-eval.n=1` over these five plus 20 and
  21 (the planner note changed). Record in this doc.

### Docs (Phase 0)

- [x] `docs/features.md` Time bullet: `[todo]` beside `[aims]` and
  `[loops]`.
- [x] `docs/architecture.md` prompt assembly order, item 8: `[todo]`
  after `[aims]`.
- [x] `docs/gantree-contract.md`: one sentence that tasks are `memory`
  rows under `todo/`, live when `superseded_by IS NULL`.
- [x] `docs/todo.md`: an "Tasks" paragraph linking here.

### Phase 1 (pendant)

- [x] `internal/memory/todo_board.go` (or in `horizon.go`):
  `TodoItem{ID int64; Slug, Text, At string}` with json tags `id`,
  `slug`, `text`, `at`; `TodoBoard(entries, loc) []TodoItem` applying
  the §4.4 rules (pattern, clip, drop empty, oldest first).
- [x] `pendant.Config.Todo func(ctx) ([]memory.TodoItem, error)`;
  `outboundFrame.Todo *[]memory.TodoItem` tagged `todo,omitempty`
  (pointer, same reason as `Aims`); `todoFrame`; `todoSent`; send on
  dial after `aims`, after `runTurn`, after `Push`, on `writeDialed`.
- [x] `cmd/gantry/run.go`: wire from the memory backend when it is
  the builtin (MCP memory has no prefix list; nil there).
- [x] Tests as in §7. Golden `testdata/todo_frame.json`.
- [x] `docs/channels.md`: dial order `cmds`, `aims`, `todo`, `allow`.
  `docs/features.md`: "and a tasks drawer on the phone".

---

## 10. Decided

| Question | Answer |
| --- | --- |
| Who owns the list | The agent. Add, update, remove are `memory_store` / `memory_forget` on `todo/<slug>`. The kernel's writes are the phone checkbox (`/todo done <id>`) and priority button (`/todo prio <id> [!!\|!]`). |
| Priority | Yes, since 2026-10-09, as `!!` / `!` leading the words. No field, no schema, no new tool. One sentence in `memory_store` says the convention ("theirs, or your read of the stakes"); one clause in the planner note says `[todo]` is in that order and the marker is the agent's to move. Every view sorts by it before age; the frame carries `priority` (omitted when normal) with the marker stripped from `text`. |
| Stale items | Never fall off. No cue on the stamp, no "still on your list?" rung. The agent keeps nagging with a different line each planner until the human says done. |
| `/todo add` | No. A kernel slug is worse than the agent's, and the agent has to live with the row. The phone's add field is plain text to the agent. |
| Due dates | The words. The planner on the named day turns them into `cron_schedule … memory_subject=todo/<slug>`. Other days do nothing. |
| Shape | Same as aims: a stamp every turn, a `/todo` view with one kernel write, a board frame the phone paints, every button a `/todo …` turn. |
