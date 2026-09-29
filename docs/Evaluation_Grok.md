# Harness evaluation

*Authored by Grok 4.7, working in Cursor, 2026-09-28. Read from the
tree, the goldens, the scenario fixtures, and the recorded live-eval
numbers in [aims-progress.md](aims-progress.md) and
[tasks.md](tasks.md). The live behavior eval was not re-run.
OpenClaw and Hermes docs were spot-checked the same day for the
"write your own" question. The opinions below are the model's.*

---

## Review

This is a good harness. I would ship it, and I would keep writing it.
It is the best single-brain crane I have looked at. The prompt is a
contract, the tool loop assumes the model will be sloppy, and the
long-horizon state lives in SQLite instead of in the model's memory.
That combination is rare, and it is the reason this repo should exist.

It is also unfinished in the place it most wants to be finished.
"Plans for months" is a real mechanism with a short proof. The daily
planner is already a 460-word policy string. And the harness has
drifted off the model it
was built for: it started on a small local model, the hardware could
not hold one big enough to keep coding against, and the work
collapsed onto Gemini Flash. The features since that collapse are
Flash-sized. The readme still says 4–30B. Those are the bad parts,
and they are specific. They do not make the harness a bad idea.

### Good

The engineering taste is good. About half the Go in this module is
tests. A stamp change shows up as a golden diff. A renamed tool fails
a fixture before the model is called. The behavior eval checks shape
— tools, args, rows, jobs — and the tasks readings show that method
working: red chat turns traced to two sentences, fixed in the
description, expects left alone. Most agent harnesses cannot say what
the model was shown. This one can.

The tool loop is good. Prefix alias, five closest names, a
grammar-constrained retry, printed-call salvage, a landing call, and
Gemini thought signatures are the unglamorous work that makes a sloppy
model finish a turn. Frontier harnesses skip it because they assume a
strong model. That assumption is the expensive one. Gantry paid for
the other one, and the repair code is still there. What no longer
matches is the prompt those repairs have to carry. That split is
under Bad.

The memory split is good. Jokes and voice in a file the operator can
delete. Facts in typed rows you can `sqlite3`. The persona outranks
recall. Rows enter because the model called `memory_store`, or because
a consolidator promoted an episode that was stored on purpose. I
trust that more than a harness that silently writes a biography of
the user every turn.

The deployment shape is good. One static binary, Distroless, nothing
listening, one channel, one model socket. Health is an exit code.
That is a product decision, and it holds up in the Dockerfile and in
the absence of any server. A compromised mouth is one container.

The restraint is good. No skills directory, no router, no control UI,
no second model call at fold, no voice API, no aim-target curve. Each
of those is a feature other harnesses advertise. Leaving them out is
why this binary is still a crane.

### Bad

The daily planner has outgrown the design. `plannerToolFirstNote` is
one 460-word string that encodes the ladder, the todo cues, the room,
ask-first, and when to stay silent. It is policy compiled into the
binary. Small models drop the middle of a note that size. The tasks
eval already caught this class of failure in a shorter description.
The goldens will not catch the next one, because they pin bytes, not
which clause was obeyed. This is the part I like least.

Follow-through is ahead of its evidence. The ledger and the kernel
math are built. The fixtures are one morning each. `planner_week_start`
passed 2 of 3 on the last n=3 reading, and the miss was the `aim_log`
the whole ledger depends on. The eval's own rule says two times in
three is a rule the persona is not carrying. Leaving that as a note
is the harness being generous with itself. A 30-day mean of `-3…+3`
scores is a mean of opinions. Useful as a read. Bad if anyone treats
it as a measurement. Tasks have the same crack at a smaller size: the
row is durable, and the due date is a sentence the planner re-reads
every morning.

An ask-first gate would make this a worse assistant. Manifest
membership is the grant. An allowlisted person with those tools
mounted is the operator, and that is the trust model a personal crane
should have. A confirm turn does not make the next action safe: if
the model asks in prose and then acts, it can still do something
other than what it just asked. If the host freezes the exact call and
waits, you have spent a turn to be asked permission to do the job.
Either way the agent becomes a nuisance. Cost controls are the real
brake: truncated results, an iteration cap, a per-server budget. A
tool you do not trust does not get mounted. I would not point this
process at someone else's inbox. I would not add `[confirm]` to fix
that. Leave the tool off.

The harness is for a local model, and Gemini Flash is the target
until a machine exists that can run one. The box could not hold a
local model large enough to keep coding against, so the daily work
moved to Flash. That was the hardware. The features written since —
the 460-word planner, the ladder, the prose due date, `[room]`
redress — fit the model that answers, and they are a lot for any
model. They work. When the unified-memory machines show up, this
prompt gets trimmed back to what a local model can carry. Until
then, Flash is the reading that counts, and the repair layer stays
because a sloppy call still happens on Flash. Pretending the 4–30B
sentence is proven today is the rot. Waiting to delete it until the
trim is honest.

Discord and Slack are unfinished vendor mouths. They cost
maintenance and they are not the product. Telegram is the same class
of option now: kept, not the mouth. The stamp has twelve tags. That
is a lot for a model, and it is working. The minimal goldens pin the
re-evaluated suffix at 221 tokens without GPS and 245 with, and the
full board at 467, so the next tag is a failing test instead of a
feeling. `/tokens` is chars/4. Fine for a fat schema. Bad as a bill.

### Unique

Three things here I have not seen done together anywhere else.

The prompt contract. Goldens from the pendant's inbound JSON through
the Completer request to the Gemini HTTP body, and the wire golden is
the same bytes the agent test pins. Other harnesses assemble a
prompt. This one diffs it. That is the feature I would steal first.

Catalog disclosure one level below skills. Hermes shows skill names
and loads the file on demand. Gantry does that to tool schemas:
`[mcp prefixes]` stays a stable on/off line, `mcp_enable` delivers the
schema on the next call, and the holds are 27 hours and 6 hours. The
tokens are in the schemas. Putting the cache boundary there is the
right layer, and I do not know another personal harness that treats
it as the product.

The ledger split. The model writes one scored event. The kernel
computes the rating, the grid, the weeks, and the slope, and stamps
them so the next morning does not re-derive them. Judgment stays with
the model. Arithmetic stays in SQLite. That is the correct cut for a
small model running a long plan, and it is further along than "the
model keeps notes in a Markdown file," which is what the gateway
harnesses actually do for goals.

Two more are unusual, with a caveat. Tool-call repair aimed at weak
models, and counted in `/toolstats`, is unusual because the field has
decided to require a frontier model instead. The repairs survived the
move to Flash. The prompt written after the move did not stay sized
for the model those repairs were for.

The mouth is unusual, and it is not a side feature. Pendant is the
default mouth: a phone PWA this project wrote, and the backend both
clients dial. Cab is a native Android app on that backend. Android
Auto is one surface of Cab, not the product. Telegram is still in the
binary, as an option. It stopped being the daily chat because of the
spam, and because a vendor messenger cannot show goals, todos, and
settings, or hand a reply to voice the way the PWA and the Android
app do. That is why
the crane stamps `[aims]`, `[todo]`, and `[room]` and ships them as
mailbox frames, and why a spoken turn comes back as `[input] spoken`
for the phone to read aloud. The glass lives in the other repos. The
reason those frames exist is this one. Face, wallpaper, and theme are
the agent dressing a client the household owns, which no gateway
harness does, because those harnesses meet the user inside someone
else's app.

What is ordinary, and fine: cron, a consolidator, FTS memory, headless
OAuth. Telegram, Discord, and Slack are optional vendor mouths. They
are not why this is good.

### Where that leaves it

Keep writing it. The good parts are structural, and they are already
in the tree. The bad parts are a long prompt and a soft gate. Fix
those. Do not add an ask-first gate, a skills library,
or a router to close the gap. OpenClaw, Hermes, and Letta are
coherent products aimed at a different job. Adopting one and deleting
their gateway is a worse maintenance burden than owning this loop.

On authorization, channel count, and agent-authored tools, gantry is
behind those projects. I think that is the correct place to be
behind. On the prompt contract, on the repair layer, and on carrying
a goal as data rather than as a note the model must reread, it is
ahead. The month-long claim has a two-morning fixture now: Tuesday
sees the note Monday's `aim_log` actually wrote, and the same line
fails. Flash is the model until the hardware catches up. The trim
belongs to that day, not to a reading nobody can run yet.

---

## What is actually here

About 29k lines of production Go under `internal/` and `cmd/`, and
about 27k lines of tests. `Dockerfile` builds with `CGO_ENABLED=0`
into Distroless. Nothing in this module calls `ListenAndServe`. One
`CHANNEL` per process. One OpenAI-compat socket. MCP children are
optional. Memory is SQLite.

The loop spends its budget on a few mechanisms that show up in tests,
not only in docs.

**The bytes the model sees are pinned.** Pendant inbound JSON, the
Completer request, and the Gemini wire body are goldens
(`internal/agent/prompt_payload_test.go`, `testdata/pendant/`). A
stamp change is a diff. `harnessTags` in `internal/agent/harness.go`
is twelve labels, and the header names only the ones present on that
turn: location, clock, hours, aims, todo, loops, progress, wakes,
surface, input, room, last contact.

**Weak-model tool calls get repaired.** Prefix alias, a closest-name
list capped at five (`maxToolSuggestions`), a grammar-constrained
retry, printed-call salvage, a landing call when the iteration budget
runs out, and Gemini `thought_signature` echoed so a multi-step cloud
turn does not 400. `/toolstats` counts the repairs.

**The catalog is disclosed by prefix.** `[mcp prefixes]` is the
on/off index. `mcp_enable` ships schemas on the next call in the same
turn. Idle holds are 27h (`ShortIdle`) and 6h (`BriefIdle`). Always-on
builtins stay. A parallel batch older than the last two tool rounds
collapses by round, so one answer of a three-call batch is not hidden
while the others remain.

**Personality and facts are split.** `SELF.md` is a file the operator
can prune, with a `:ro` kill switch. Voice bits graduate on fold.
`/new` distills. Facts go to typed SQLite rows through `memory_store`,
or through the consolidator promoting an episode that was stored on
purpose. GPS is in-memory and dies on restart. There is no embedding
index.

**Aims and tasks have a kernel side.** The agent writes `aim_log`.
The kernel keeps the event, the per-aim scores, the blocks, and the
derived rating, grid, weeks, and slope. Tasks are `todo/<slug>` memory
rows, stamped with ids, closed by the human (`/todo done` or
`memory_forget`). Both can ride the pendant mailbox as frames. The
daily planner is one clock time. A quiet day is `[silent]`.

**Behavior is a second contract, and it costs money.** Twenty-seven
fixtures under `internal/agent/testdata/eval/`. Two of them
(`08_denver_flight`, `11_rental_daily_cron`) pull a real MCP catalog
at run time and call nothing. The gate checks shape: tools, args, call
counts, `[wait]`, `[silent]`, rows, jobs, prices that must have come
from a tool, ledger lines that must share words with a tool result or
the human. `round_budget` is printed, not failed. It runs on demand
and in front of a release, not on every push. Recorded readings are
`gemini-3.6-flash`.

**The mouth is Pendant.** The phone PWA is the default client, and its
Worker is the backend. Cab is a native Android app on that backend.
Android Auto is one surface of Cab. Telegram remains an option, kept after
spam and after a vendor chat could not show goals, todos, or
settings, or pipe a reply to voice. This binary stamps `[room]` and
can call `pendant__avatar_update`, `backdrop_update`, and
`theme_update`. Aims and todos leave as mailbox frames. A spoken turn
is stamped `[input] spoken` so the phone can read it back. The
picture path, the theme catalog, and the fan-out live in pendant,
cab, and the mailbox worker. The crane is what makes those screens
part of the harness.

---

## What is load-bearing

These are the choices that make the binary worth keeping. Reversing
them to look more like a gateway harness is the change that makes
this repo a mistake.

- **No inbound port and no UI in this process.** Health is an exit
  code. The yard is gantree. Distroless fits because nothing here
  needs a shell or a listener.
- **One socket, one brain.** `thought_signature`, grammar retry, and
  `LLM_SYSTEM_FOLD` are already per-model glue on a single route. A
  router inside the unit multiplies that glue and puts every
  household's failover on one process. Model variety belongs across
  units, each with its own `.env` and its own blast radius.
- **Procedure lives in the tool description.** A skills file that
  restates `google__calendar_list_events` is a second copy of the
  manifest. `skill/<area>` memory rows are the right size for the
  exception a schema cannot say.
- **One model call at fold.** The fold writes `Facts:` and `Voice:`.
  A silent "store everything now" turn before compaction stores what
  the model just chose not to store. A per-turn synthesis of "who
  this person is" re-derives a paragraph the history and the standing
  summary already hold.
- **Voice stays on the phone.** The harness sees text. `[surface]`
  and `[input] spoken` ask for a few short plain sentences. An
  STT/TTS bill on every turn buys the same experience the device
  already has.
- **The operator can delete a line of personality, and can `sqlite3`
  a fact.** Jokes in a file, facts in rows. Recalled rows do not
  outrank `PERSONA.md`.
- **A tool batch is read back whole.** Hiding one result of a
  parallel round is how a small model invents the missing number.
- **The agent scores an event; the kernel counts.** One night out is
  one row scored against every aim it touches. A target curve per aim
  becomes an argument about whether pizza is `-1` or `-3`. The rubric
  stays with the model. Means, buckets, slope, and a correlation stay
  in SQLite.
- **A fixture miss is a sentence to fix.** The tasks readings are the
  proof: chat turns that filed "call the dentist" as `follow/` and
  opened with `memory_recall` went green after description edits, with
  one expect loosened to match the written contract. That method works
  when a miss is treated as a miss.

---

## Where it is thin

Ranked by how much they threaten the reason to keep the repo.

### 1. The planner note is a constitution

`plannerToolFirstNote` in `internal/agent/agent.go` is one string,
about 2,600 characters, about 460 words. It tells the daily turn how
to review prefixes, batch tools, score `aim_log`, climb the ladder,
ask first, treat an empty calendar, redress `[room]`, emit `[wait]`,
read week lines, and cue todos, including the ban on offering to drop
one. It sits at the end of the prompt, where recency makes it the
standing policy, and it grows every time a feature needs the morning
turn to behave.

The tasks eval already showed what that costs. Two sentences sitting
next to each other in `memory_store` decided whether a chore became
`todo/` or `follow/`. A 460-word note will keep producing that class
of miss. Small models drop the middle. The goldens stay green,
because they pin the bytes of the stamp, not which clause the model
obeyed.

A new behavior that needs another sentence in that const is too big
for the planner turn.

### 2. Follow-through is one morning

The ledger and the kernel math are real. What the fixtures prove is
narrower.

- Each planner fixture is one turn against one seeded state.
- `planner_week_start`, on the 2026-09-26 `gemini-3.6-flash` n=3
  read in [tasks.md](tasks.md), passed 2 of 3. The miss was no
  `aim_log`. The same file records it at 3.67 rounds and about 32k
  prompt tokens. The 2026-09-26 n=1 read in
  [aims-progress.md](aims-progress.md) has that fixture at 4 rounds
  and 34.9k. The six ladder fixtures the day before averaged about
  3.1 rounds and 23k, with 10 of 18 runs over their round budget.
- The eval harness comment says a rule that holds two times in three
  is a rule the persona is not carrying. This fixture was left as a
  note.
- There is no fixture that runs Monday, stores the note the model
  actually wrote, and runs Tuesday.

A five-day grid computed from seeded rows proves the arithmetic. It
does not prove the ladder across a week, or that Tuesday's line
differs from Monday's. A 30-day mean of `-3…+3` scores is a mean of
opinions. The measurement trend (a weight the tool returned) is the
meter. The score is the read. Those are different objects, and the
product has to keep them apart or the mean becomes a fake instrument.

Tasks repeat the pattern at a smaller size. The row is durable, it
does not age out, and only the human closes it. The due date is the
words. "Wed 11am" is re-parsed every morning by the same model that
missed `aim_log`. Nothing in the kernel notices a misread. A pocket
list of memory rows is a sound shape. A planner-as-parser will miss
Thursday.

### 3. Ask-first is the wrong fix

Cost controls are in the host: truncated results, an iteration cap,
per-server `budget`, refusals that name the reset. The grant is the
manifest. An allowlisted chat with tools mounted is the operator.
The planner also carries the sentence "Ask first before sending
mail, spending, or posting," which is the nuisance written down.

I had this ranked as a hole, and recommended a harness `[confirm]`
that holds send, spend, and post until the human's next turn.
`guardEnable` could do it. That was the wrong recommendation. An
assistant you do not trust to act is a form. Asking first wastes the
turn the assistant exists to spend, and it does not pin the action:
a prose question and a later tool call can disagree. Freezing the
exact call and waiting pins the bytes and still makes the agent ask
permission to book, send, or post. OpenClaw's approval stack fits a
gateway that runs commands for people who did not mount the tool.
This crane is one person and the tools they chose. If a tool is too
dangerous, it stays out of `mcp.toml`. The sentence in the planner
note should go too. It spends attention on a behavior the product
refuses.

### 4. Vendor mouths are still in the tree

Deploy is one channel per process, which is the right blast radius.
The mouth this project runs is Pendant, with Cab on that mailbox.
The source tree still also contains Telegram, Discord, Slack, and
stdio. Telegram has the menu, photos, reactions, pin, and error tee.
Discord and Slack are thinner. The env default is still `telegram`
(`internal/config/config.go`, the readme, `.env.example`). That
default lags the mouth. A fresh boot that does not set `CHANNEL`
still comes up as the vendor chat they left.

Vendor socket work does not get more valuable as the crane gets
better. Pendant and Cab earn their keep: a goals board, a todo list,
settings, and voice, on the PWA and in the Android app. Android Auto
is one surface of that app.
Telegram earns a kept option. A second and third vendor chat earn
theirs only when someone is actually on them.

### 5. The stamp is the feature, and it is a lot

Time, place, hours, aims, todos, loops, and wakes without a tool
round are why the planner can plan. They are also twelve tags, and
`[progress]` makes the planner the fattest turn in the recorded
readings, from the low 20ks to the mid 30ks. Caps of five aims, five
tasks, five loops, and five grid days stop a silent truncation.
`[input]` is already an instruction ("read aloud, no markdown") on
every spoken turn, which is product copy riding the stamp. It works.
It is a lot for a model.

`volatile_est_tokens` is logged on the live model call, and `go test`
now fails when the re-evaluated suffix moves. The minimal pendant
goldens pin 221 without GPS and 245 with. The full board pins 467.
A thirteenth tag has to be worth those tokens.

`LLM_SYSTEM_FOLD=many` is the default off Gemini, and it is unverified
against a local chat template that renders system text only at
position 0. That check waits for the machine. Flash is the target
now, because that is the model the hardware can run and the model
the new features were written against. The local trim is a later
job: cut what that model cannot see, once it exists. Another ledger
column written on Flash is still scope, and the size pin is how you
notice it.

### 6. Token accounting is chars/4

`/tokens` labels the estimate. Native `usage` is kept when the
provider sends it. That is enough to notice a fat schema. It is a
poor bill, and a poor trend across Gemini and a local tokenizer.

---

## Should this exist?

Yes, if the product is a fleet of single-brain boxes: Distroless,
outbound only, one person, one model, a phone and a native Android
app the household owns, memory you can open with `sqlite3`. That product is
not a config flag on OpenClaw or Hermes. Their center of gravity is a
long-running gateway, many credentials, skills the agent writes, and
a model strong enough that tool-call repair is someone else's
problem. OpenClaw's current docs describe exec approvals, session
permission modes, and plugin permission requests. Hermes maintains a
skill library with a curator; the expensive consolidation pass is
opt-in, and prune is the default. Those are coherent products. They
are the shape this binary refuses. Ripping that shape out and
rebasing it every upstream release costs more than owning this loop.

A personal assistant for this quarter is a different job. A gateway
harness plus an allowlist gets mail, calendar, and chat with less of
this loop, and you live with their memory story. The work already in
this tree is then a research project. That is allowed. It is a bad
description of operations software, and it is a bad reason to grow
channels.

Leave these on the table. They look like gaps next to the other
harnesses. They are the product boundary.

- A skills marketplace, or a curator that writes `SKILL.md` after a
  session.
- An in-process model router.
- A control UI in this binary.
- A voice vendor on the turn.
- A "who this human is" block rebuilt every prompt.
- A typed aim target, a grade table, or an analytics model call.
- Subagents inside the unit. One brain is the blast radius. The fleet
  is how it scales.
- Auto-save of every turn. A harness that stores each exchange as an
  episode teaches the consolidator to promote hallucinated emails at
  confidence 1.
- Another vendor channel.
- An ask-first gate. A tool you do not trust stays out of the
  manifest. A confirm turn makes the assistant a form.

Done, because the tree's own evidence asked for them.

1. **`planner_week_start` fails without `aim_log`.** The expect names
   the tool, and a scripted week line with no log fails
   `TestEvalHarness_WeekStartRequiresAimLog`. The canned Garmin result
   is a new weigh-in (`weight 183 lb`), so logging it is the turn,
   not a guess about a number already on the ledger.
2. **`26_planner_two_mornings`.** Monday and Tuesday share one
   session. Tuesday's stamp carries the `praised` note Monday's
   `aim_log` wrote. The same line fails. `TestEvalHarness_TwoMornings`
   covers both the carry and the repeat.
3. **Stamp size is pinned.** `VolatileEstTokens` on the minimal
   goldens is 221 without GPS and 245 with. The full board is 467.

Still open, and not for this week:

4. **Flash is the target.** The harness is for a local model. The
   trim happens when a machine with the memory to run one exists.
   Until then, do not pretend a 4–30B reading has been taken, and do
   not delete the sentence as if the destination changed.
5. **Twelve tags is a lot, and it is working.** The pin in (3) is the
   alarm. A thirteenth tag has to be worth the tokens it adds.

A `task` table, priorities, projects, and a done ledger can stay
absent. The pocket list is small on purpose. The fragile part is the
prose due date. If that gets a fix, the fix is a time the human
confirmed ("make that Thursday" already rewrites the row), stored so
the planner does not re-interpret the sentence. That is a column. It
waits until the two-morning fixture exists. Done-history can stay
absent. "How many did I clear this month" can stay unanswered.

This harness should exist. It should stay a crane. The next change
that makes it a gateway is the change that makes the custom binary
the wrong one.
