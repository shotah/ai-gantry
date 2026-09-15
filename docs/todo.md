# Todo

Open work. A line is done when tests say so (`go test` on the package
you touched). Mouths: [channels.md](channels.md). Prompt order today:
[architecture.md](architecture.md#prompt-assembly-order). Sibling
lists: pendant [todo.md](https://github.com/shotah/gantry-pendant/blob/main/docs/todo.md)
· [audit_todo.md](https://github.com/shotah/gantry-pendant/blob/main/docs/audit_todo.md)
· cab [todo.md](https://github.com/shotah/gantry-cab/blob/main/docs/todo.md).

---

## Pendant draft stutter (`bo` … 10 s … full reply)

Seen on the PWA and Cab: the Kit bubble paints the first delta
(`bo`), freezes, then the whole reply lands ~10 s later. Traced from
the pendant checkout. **This repo. Not the mailbox, not the
Cloudflare plan, not the 429s.**

### Where it is

`internal/channel/pendant/stream.go` `pushLocked`:

```go
if !force && s.lastFlushed != "" && time.Since(s.lastFlushAt) < streamMinGap {
    return nil // dropped — nothing ever re-sends it
}
```

`streamMinGap` is 1 s and the throttle is **leading-edge only**. A
delta that lands inside the gap is dropped and no timer flushes
`s.latest` later. So:

- Steady tokens → at best one draft per second. That is the stutter.
- Fast model (whole answer inside ~1 s of the first delta) → only
  the first delta ever paints. The bubble sits on `bo` until
  `Finish` writes the `reply`.
- Between end-of-stream and `Finish` nothing paints: theater nudge
  (a second Completer call), tool rounds (silent unless
  `TOOL_TRACE` is on), `finishText`, `sessions.Append`. That is the
  10 s. `typing` every 4 s is the only heartbeat.

Telegram's `editStream` does not have this: `flushLoop` ticks every
`streamFlushEvery` and sends `latest` whenever it differs from
`lastFlushed`, so the trailing edge comes free. Pendant skipped the
loop because a draft is one WebSocket frame, and lost the trailing
edge with it.

### Not the mailbox, not $5

- Worker `draft` path (`worker/mailbox.ts`): one cached `roomUsers`
  read and a fan-out. No `seq`, no queue row, no transcript write,
  and exempt from the 30 frames/min bucket (`cranePublishedDraft`
  skips `take`).
- Workers Free vs Paid changes quotas, not latency. DO CPU per
  WebSocket message is 30 s on both. Free caps (100k req/day with
  WS messages billed 20:1, 13k GB-s/day, 100k row writes/day) fail
  with an error when hit; they do not slow down.
- The Worker's only 429s are `/api/auth/*` and `/api/push` (40/min
  per IP, per isolate). `/ws/` never 429s; a refused chat frame is
  an `error` `rate` frame, which drafts skip.

### Debounce, or wait for more of the answer?

Neither. Debounce (wait for quiet) is the wrong shape: tokens do not
go quiet until the end, so it degrades to "paint once at Finish".
Waiting for N chars gives bigger, rarer jumps. The bubble wants a
**throttle with a trailing edge**: at most one draft per gap, and
always one more after the last delta. A draft is cheap, so the gap
can be short.

### This checkout (`pendant/stream.go`)

- [x] Trailing flush in `pendant/stream.go`. When `pushLocked`
      suppresses a push, arm `time.AfterFunc(streamMinGap - since)`
      that re-runs the flush for `s.latest` if it is still newer than
      `lastFlushed`. Invariant: a draft is never more than one gap
      stale.
- [x] Stop that timer under `s.mu` in `Finish` and `Discard`
      **before** the `reply` / empty `draft` goes out. Guard the
      callback with a `finished` flag, not just `timer.Stop()`.
- [x] `streamMinGap` 1 s → 250 ms. Mailbox cost is nil (no storage,
      no rate tokens; a 10 s answer at 4 Hz is 40 WS messages = 2
      billable DO requests).
- [x] With the trailing flush the streamed text is at least complete
      on the phone while a nudge / tool round runs. If the silence
      still reads as frozen, an `UpdateStatus` line during a
      `TOOL_TRACE`-off tool round is already force-flushed.
- [x] Finalization hitch after a complete draft: crane `Finish`es the
      stream **before** `sessions.Append` / wait cron. Worker fans
      `reply` to live sockets **before** queue+transcript writes.
      PWA keeps a stable `kit-live` key so promoting the draft does
      not remount the bubble.
- [x] Log mailbox `error` frames instead of dropping them
      (`ignoredKind` in `pendant.go`). A rate-refused `reply` is lost
      silently today. `typing` every 4 s costs 15 of the crane's 30
      frames/min, so two concurrent turns can starve a `reply`.
- [x] Tests: `TestEditStream_UpdateThrottled` — `streamMinGap = 20ms`,
      two quick `Update`s, no further calls → a second draft `Hello`
      arrives within a few gaps. Still one draft per gap. 
      `TestEditStream_FinishCancelsTrailingDraft`: `Finish` right
      after a suppressed `Update` yields no `draft` after the `reply`.
- [ ] Verify on a real turn from the crane log: `model call`
      (`first_token_ms`, `dur_ms`, `iteration`), `tool call` /
      `tool done` (`dur_ms`), and `model narrated tool action in
      prose without calling` (a nudge round). That attributes the
      10 s.

### Sibling (mouths)

Worker + PWA were on the **finalization** hitch, not the draft
cadence. Live `reply` used to `rememberPhone` before `send`, and
promoting `__draft__` → a new id remounted markdown. Closed in
gantry-pendant this round (fan first; stable `kit-live` key). Cab
still keys the draft by `DRAFT_ID` then a new reply id.

Pendant may still exempt `typing` from the crane rate bucket the
way `draft` is; that is `worker/mailbox.ts`, not here.

---

## User-role clock leak

Kit (the running agent) called this out: a **context auto-hydration
block** sat in the Completer's **raw user message** again this turn,
and asked to "strip context header from client message body on send"
(pendant `audit_todo.md`).

**Where it actually is: this repo, not the mailbox.**

`internal/agent/agent.go` `runTurn` used to do:

```text
userMsg.Content = storeText + "\n\n" + clock
```

`clock` is `[location]` (from `here`) + `[current time]` (week grid,
day-part, tz sermon) + `[hours]`. That is prompt-only — SQLite history
does **not** store it (`TestHandle_PendantGPSLeadsClockFooterOnUserTurn`).
The model still saw it as **the human's words** this turn. **Closed:**
RoleUser is speech; `[harness]` RoleSystem after it holds the clock.

`[memory]` hydration is a **system** message (after history, for the
prompt cache). Different block. Do not fold it into `Text` either.

### Not pendant BE (Worker)

The Durable Object forwards `text` and `context` as separate JSON
fields (`lib/mailbox/frame.ts`). It must **not** merge GPS / battery /
tz into `text`. Do not "fix" this by rewriting bodies in the Worker.

### Mouths (open in those repos)

Happy-path `"near me"` without `[location]` in `text` is **not** the
Cab ticket. Cab still strips a pasted header **on send**. PWA is done.

| Mouth | Repo | Ticket |
| --- | --- | --- |
| PWA | gantry-pendant **FE** | closed (`stripHarnessContext` + geo-only `context`) |
| Cab | gantry-cab | `docs/todo.md` Small |
| Worker | gantry-pendant **BE** | no work |

### This checkout

- [x] Completer **RoleUser** `Content` is `storeText` only (what they
      typed / `[photo]` / steers). No `[location]`, `[current time]`,
      `[hours]` in that string.
- [x] Clock / location / hours stay **prompt-only**, tagged, recency-
      weighted. Do **not** persist them in `session` (already true —
      keep the pin_test history assertion).
- [x] Small-model gate: do **not** put `[current time]` *before* their
      words (calendar/tool fixation). Tagged **`[harness]` RoleSystem
      after** RoleUser (not unlabeled trailing system, not leading
      time). Gate: `internal/agent/pin_test.go`,
      `TestAgent_Handle_MemoryHydration`.
- [x] If inbound `Message.Text` already contains those tags (paste /
      old client), strip before store and before hydrate FTS
      (`stripHarnessContext`).
- [x] `docs/architecture.md` prompt order matches the loop (hydration
      after history; `[harness]` clock after user speech).
- [x] Completer payload is pinned, not hypothetical: PWA inbound JSON
      → `pendant.InboundTurn` → `Handle` → `testdata/pendant/completer_*.txt`.
      Time and location live on `[harness]` after RoleUser, not in `text`.
      Phone `context.at` / `tz` are ignored. GPS-off still has `[current time]`.
- [x] Gemini OpenAI-compat mismatch: trailing `[harness]` system after
      the user does not land in `system_instruction`. `WireMessages`
      folds it into the one system message (`completer_geo_gemini_wire.txt`).
      OpenAI/Ollama keep the trailing system.
- [x] `[harness]` also stamps `[hours]` (always, when memory is on),
      `[aims]` (live `aim/<area>` insight), and `[loops]` (`waiting/` /
      `follow/` facts). North-stars stay in `SELF.md`. No new `goal` kind.
- [x] Phone `context.at` / `tz` / `battery` / `net` stay ignored (crane
      clock wins). `context.surface` is now read (closed set) → `[surface]`;
      see the harness stamp section below.

### Sibling (mouths — not this tree)

PWA send strip is closed in gantry-pendant (`stripHarnessContext` +
geo-only `context`). Cab still owns its Kotlin strip (`docs/todo.md`
Small). Do not implement Cab from this checkout.

---

## Harness stamp (`[harness]` block)

Landed: RoleUser is speech; one tagged `[harness]` RoleSystem after it
carries `[location]`, `[current time]`, `[hours]`, `[aims]`, `[loops]`,
`[wakes]`, `[surface]`, `[room]`, `[last contact]`. Gemini folds it into the one
system instruction (`provider.WireMessages`, `LLM_SYSTEM_FOLD`). Goldens:
`internal/agent/testdata/pendant/completer_*.txt` — the agent Request,
the Gemini wire, the horizon block, and the full board. In
`completer_geo_gemini_wire.txt` the line `You are Kit.` **is** the fixture
persona — nothing is missing. `go test ./internal/agent/ -run Payload
-update` rewrites goldens; read the diff first.

### Fixes

- [x] Gemini fold order. Standing (persona, summary, `[mcp prefixes]`,
      `[memory]`) first, this-turn (`[harness]`, wait note, talk footer)
      last. Identity leads, stable prefix stays cacheable, clock has
      recency. Gate: `TestWireMessages_GeminiFoldsHarnessIntoSystem` +
      `*_gemini_wire.txt`.
- [x] `[loops]` starved `follow/`. `FormatLoops(waiting, follow)`
      interleaves so five open waits cannot hide a follow. Gate:
      `TestFormatLoops_FollowSurvivesFiveWaits`.
- [x] Silent truncation. `[aims] … (+3 more — memory_recall aim/)`,
      `[loops] … (+N more — memory_recall waiting/ follow/)`.
      `ListBySubjectPrefix` default window is 30 so the count is honest.
- [x] Age on aims/loops. `(12d ago)` from `updated_at` after the first
      day (`channel.Age`, the same helper `[location]` uses). Loops past
      three weeks add `— resolve or memory_forget`. Gate:
      `TestHorizonAge_StampsDaysAndStaleCue`.
- [x] `[hours] unknown` nagged forever on `MEMORY_BACKEND=mcp`.
      `MCPAdapter.ActiveByKindSubject` returns `memory.ErrNotSupported`;
      `hoursStamp` skips the line. Gate:
      `TestHandle_HarnessSkipsHoursOnMCPBackend`.
- [x] Spark prompts read the stamps. `sparkToolFirstNote`, the spark
      nudge, `cron.SparkPingPrefix`, `cron/spark.go` pool,
      `selfnote/stamp.go` "Empty board" all say `[hours]` / `[aims]` /
      `[loops]` / `[wakes]` are in `[harness]`; `memory_recall` only for
      detail; `cron_list` only for the full audit. Gates:
      `TestSparkPrompts_ReadHarnessNotRecall`,
      `TestSparkNotes_ReadHarnessNotRecall`.
- [x] Hydration double-stamps. `horizon.dropStamped` removes rows already
      on `[aims]` / `[loops]` from `[memory]`. Gate: full-board test
      (`pref/food` hydrates, `aim/training` does not repeat).
- [x] `LLM_SYSTEM_FOLD=auto|one|many` (`provider.WireMessagesMode`,
      `Client.WithSystemFold`). `auto` keeps the `gemini*` guess.
- [ ] Check each local template you run with `LLM_SYSTEM_FOLD=many`
      (default for non-Gemini). Ollama Gemma renders system only at
      position 0, so a trailing `[harness]` may be dropped — if the model
      cannot say NOW without a tool, set `one`. Needs a live Ollama, not
      a unit test.
- [x] `[harness]` header names only the tags present (`harnessNote`).
      Gate: `TestHarnessNote_NamesOnlyPresentTags` + every golden.

### Additions (same pattern, data already in SQLite)

- [x] `[wakes]` next ≤3 enabled once/daily/every jobs for this session
      (`cron.FormatWakes`; spark, examples, wait pokes skipped). Wired via
      `agent.Options.Wakes = cronStore`. `(+N more — cron_list)`.
- [x] `[surface]` from phone `context.surface` (closed set: browser,
      android, android_auto, ios, carplay). `android_auto` / `carplay`
      add the shared read-aloud hint (`spokenHint`: like a person, a few
      short sentences, no markdown / lists / code / links / emoji).
      Battery / net stay dropped. Fixture: `inbound_cab_auto.json`.
- [x] `[input]` from phone `context.input` (closed set: `spoken` —
      pendant hold-to-talk, the pocket reads the reply via `/api/tts`).
      Same `spokenHint` as the car; bare `[input] spoken` when the
      surface is already driving so the model is not told twice.
      Fixture: `inbound_pwa_spoken.json` → `completer_spoken.txt`.
- [x] `[last contact]` from `session.Store.LastUserAt` (cron rows
      ignored): `last human message 3h ago (Mon 2:15 PM)` or `none in this
      session — first message`. Optional History capability; test fakes
      without it stamp nothing.
- [x] `[room]` from the pendant channel's cached `face` / `backdrop` /
      `theme` notices (the mailbox already sent them to the crane socket;
      `ignoredKind` used to drop them). Stamped only when `pendant__*` is
      in the catalog; prefix off this chat → `pendant is off this chat`
      instead of the nudge. State plus one clause — no tool recipes on
      the stamp, the spark note, or the `Room:` pool line; the tool
      descriptions carry the how. Wired via `ag.SetRoom(ch)` when the
      channel implements `agent.RoomSource`. Gates: `TestRoomStamp_*`
      (including a no-recipe check), `TestNoteRoom_*`, full-board golden.
- [ ] `[room]` after a crane restart reads `theme not seen since boot`
      until the next notice — the Worker flushes theme to phones on
      connect (`flush`, role `phone`), not to the crane. Three-line change
      in gantry-pendant `worker/mailbox.ts` to send the stored theme /
      backdrop rev / face rev to a `crane` socket too; old cranes already
      drop those kinds.

### Test gap

- [x] End-to-end wire body: `prompt_wire_test.go` runs `Handle` through a
      real `provider.Client` into `httptest` and diffs the JSON body
      against `completer_geo.txt` (OpenAI name) and
      `completer_geo_gemini_wire.txt` (Gemini name) — the same goldens
      the agent-side test pins, byte for byte.

### Not harness

Weather, calendar, mail are live data — tools, not stamp. Battery and
net change no behaviour. Keep the block boring and bounded.

**No `[from]` / speaker stamp.** One person, one mouth. Multi-user
session logic was stripped from this tree once already; do not add it
back as a harness line or anywhere else.
