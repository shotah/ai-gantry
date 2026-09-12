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

### Sibling (not this tree)

Nothing in the Worker, PWA, or Cab for the stutter. Pendant may
exempt `typing` from the crane rate bucket the way `draft` is; that
is `worker/mailbox.ts`, not here.

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
- [ ] Phone `context.at` / `tz` / `surface` / `battery` / `net` —
      PWA no longer stamps them (geo only). Old Cab may still send
      them. Unused here except `geo` → `here`. Not this ticket.

### Sibling (mouths — not this tree)

PWA send strip is closed in gantry-pendant (`stripHarnessContext` +
geo-only `context`). Cab still owns its Kotlin strip (`docs/todo.md`
Small). Do not implement Cab from this checkout.
