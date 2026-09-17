# Draft stream — pendant / crane handoff

Written in the Cab checkout (`gantry-cab`) on 2026-09-17 for the
mailbox (`gantry-pendant`) and crane (`ai-gantry`) checkouts. Copy
into `gantry-pendant/docs/`. Cab's own half is at the bottom and is
not a blocker for either of you.

**Symptom on the phone.** Ask Kit something. The typing chip shows,
the streaming draft paints, then the draft **vanishes**. Nothing for
many seconds. Then the finished `reply` lands. Other times the same
question streams and finalizes cleanly. Cab-side trace below; the
Cab paths are exhausted, so the rest is wire and Worker.

Wire: pendant `docs/frontends.md`. Sibling fan:
[sibling_phones.md](sibling_phones.md). Cab's draft code:
`Mouth.kt` (`applyDraft`, `commit`, `setUp`), `MailboxClient.kt`.

---

## What Cab does with the stream today (do not guess)

| Frame / event | Cab | Number |
| --- | --- | --- |
| `typing` | chip on; off after TTL with no refresh | `TYPING_TTL_MS` 6 s (crane refresh ~4 s) |
| `draft` with text | replace the one draft bubble, always sorted last | — |
| `draft` with **blank or missing `text`** | **remove** the draft bubble | — |
| `reply` with text or image | replaces the draft (same Compose key, no remount) | — |
| `reply` with **no text and no image** | **remove** the draft; paint nothing | — |
| `error` | mark your bubble failed; chip off; draft untouched | — |
| socket down (`onClosed` / `onFailure` / stop) | `setUp(false)` → **remove** the draft; chip off | retry 2 s → 60 s |
| socket up | `ack since:<seq>`, take the replay; draft is **not** restored unless a new `draft` arrives | — |
| OkHttp protocol ping | every 20 s; **fails the socket** if no pong in 20 s | `pingInterval(20 s)` |
| sweep | second socket while the thread is on screen; first socket `cancel()`ed once the second is open | every 2 min, and on resume, min gap 10 s |
| connect flush replay | stored `inbound` / `reply` / `push` with `replay: true`; restamps ids already painted | — |

Drafts are never stored on the phone. `ThreadCache` and hydrate drop
them. A draft that leaves the screen only comes back as a new `draft`
frame or as its `reply`.

## The three ways a draft leaves before its reply

1. **A socket gap.** Any close or failure mid-answer clears the draft
   (Cab, by design — reconnect was "start clean"). Reconnect + replay
   brings stored frames, not the in-progress draft. If the crane
   sends no more drafts (finishing, or inside a tool call), the
   bubble stays gone until `reply`. Causes: network blip; a late
   protocol pong; the Worker closing the socket.
2. **A blank `draft`.** Cab reads missing / whitespace `text` as
   "clear." If the crane resets its buffer at a tool call, or the
   Worker's coalescer flushes empty, the bubble vanishes.
3. **An empty `reply`.** No text, no image → Cab drops the draft and
   paints nothing.

How to tell in the field: if the header loses **Live** or shows
"Mailbox socket down — …" at the moment the draft goes, it is (1).
If the header is calm and the PWA loses the draft at the same
instant, it is (2) or (3).

---

## Pendant (`gantry-pendant`) — Worker

Ordered by payoff. All additive; an old APK sees only frames it
already knows. Done in `worker/mailbox.ts`; contract in
[frontends.md → Draft](frontends.md#draft-what-every-mouth-must-do-the-same).

- [x] **Fan-out past a stale socket.** Found while checking (1):
      `getWebSockets` still lists the socket Cab `cancel()`ed (no
      close frame → CLOSING until the peer answers), and `send()` on
      it throws. Every fan loop was unguarded, so a stale sibling
      ahead of the live one aborted the loop and the `draft` /
      `typing` / `reply` never reached the phone. That is (1) with a
      calm header. Every fan now goes through `fanOut` (open filter +
      per-socket try/catch); the crane queue decision counts sockets
      that actually took the frame.
- [x] **Re-send the in-progress draft on a phone connect flush.**
      Latest `draft.text` per `sub` in DO memory (`drafts` map, no
      storage write per token). Connect flush: transcript, unread
      queue, then one plain `draft` (no `replay`, no `seq`). Dropped
      on that human's `reply` / `error`. If the room hibernates
      mid-stream the phone waits for `reply` as before.
- [x] **Blank-draft semantics: (b).** The Worker drops a `draft` with
      missing / whitespace `text` before fan-out and it does not
      touch the held draft. The PWA already read blank as "clear"
      (`PhoneShell` `onmessage`, `draft` branch drops the
      `__draft__` bubble when `text.trim()` is empty), same as Cab;
      that path no longer fires from the wire.
- [x] **Never fan a `reply` with no text and no images.** Refused as
      `error bad frame` back to the crane with the frame `id`;
      nothing fanned, stored, queued, or pushed. Photo-only `reply`
      still passes.
- [x] **Socket close bookkeeping.** Checked: no per-`sub` singleton,
      no "last socket" map, no sibling closes. `webSocketClose` /
      `webSocketError` only `close()` the one socket. The stale-socket
      fan was the only way an abort hurt a sibling.
- [x] **`typing` off the crane rate bucket.** Not in the brief, but
      the crane item below (refresh ≤ 4 s through a tool call) would
      spend the 30/min bucket on chips and bounce the `reply` as
      `rate`. `typing` now skips `take()` like `draft` already did.
- [ ] **Protocol ping / pong under load.** Cab fails the socket if a
      WebSocket-level pong is 20 s late. Hibernation API handles
      protocol pings in the runtime; still verify on a long answer in
      the Walk (5), not by reading docs. The text `ping` / `pong` the
      mailbox already does is separate and fine.
- [x] **`frontends.md`.** Draft section: cumulative full text, blank
      dropped, empty reply refused, connect flush may end with one
      `draft`, fan-out guard, `typing` rate.

### Tests (`test/worker/mailbox.test.ts`)

Fake `DurableObjectState` (storage map, tagged sockets that stay
listed after `close()` and throw on `send()` like workerd).

- Connect flush for a `sub` with an answer in progress emits the
  stored transcript, then one `draft` with the current text; a `sub`
  with no answer in progress and an anonymous socket get no draft.
- `reply` / `error` for that `sub` clears the held draft; the next
  connect gets none.
- Blank `draft` dropped; does not clear the held one.
- Empty `reply` refused as `bad frame`; storage untouched; photo-only
  passes.
- Two phone sockets, same `sub`; first half-open (or throwing while
  OPEN); a following `typing`, `draft`, `reply` reach the second.
  Same for a room notice and for the crane queue decision.
- 40 `typing` + 40 `draft` then `reply`: no `rate`; 31 replies: `rate`.

---

## Crane (`ai-gantry`)

Checked 2026-09-17 against `internal/channel/pendant/{stream,pendant}.go`.
All three already held; each is now pinned by a test so it stays that
way.

- [x] **Tool calls do not blank the draft.** `editStream` keeps one
      cumulative bubble: a finished segment is committed to `body`, the
      live segment is `answer`, and every `draft` is `body + answer`.
      `pushLocked` never writes empty text; an `Update("")` at the
      start of a tool round is a no-op. The only blank `draft` the
      crane ever sends is `Discard` (cancel / empty turn / error), and
      the Worker now drops those. Pinned:
      `TestEditStream_ToolRoundKeepsDraftWhole`.
- [x] **Keep `typing` alive through a tool call.** `startTyping` is a
      ticker for the whole `Handle`, not per model call; a tool call is
      just a slow `Handle` to it. `typingInterval` is 4 s. Pinned:
      `TestDispatch_TypingRefresh` (handler blocked, refreshes keep
      coming). If the chip still drops on a long tool call in the Walk,
      it is the wire (Worker rate bucket / stale-socket fan, both fixed
      above), not the cadence.
- [x] **No empty `reply`.** `replyFrames` returns nothing for blank
      text with no image; `Finish` and the non-stream path write no
      frame. `writePush` likewise sends nothing for blank text (it used
      to fall through to a text-only frame). Pinned:
      `TestEditStream_FinishEmptyWritesNothing`,
      `TestPush_EmptyTextWritesNothing`.

Also fixed while here: a `Handle` error now lands as a short `reply`
(`channel.HandleFailedText`) after the draft is discarded, so a
provider 400 is not "draft vanishes, then nothing." Pinned:
`TestDispatch_HandleErrorTellsHuman`.

Also fixed, not in the brief: the `reply` was written only on the
socket captured at dispatch. If the crane's own mailbox socket dropped
mid-turn (`close 1006` in the logs), `Finish` failed and the reply was
lost — drafts stop, chip expires, nothing lands. `reply` frames (final,
error line, non-stream) now take the same fresh-dial fallback `push`
already had (`writeReply` → `writeDialed`). Drafts and `typing` do not
dial; they are ephemeral and the serve loop replaces the socket once
`Handle` returns. Pinned: `TestDispatch_ReplyFallsBackToDialWhenSocketDies`
(stream and non-stream; exactly one dial).

---

## Cab (`gantry-cab`) — this checkout, pending

Not a blocker for the two above. Cab will stop clearing the draft on
a socket gap: `setUp(false)` leaves the draft; the reconnect replay
delivers the `reply` (replaces it, as today) or an `error`; a draft
with no update for ~60 s expires so a dead crane does not leave a
ghost. `MouthTest`: draft survives down → up; replayed reply replaces
it; stale draft expires. With the Worker re-sending the draft on
connect, both fixes overlap on purpose — belt and braces, same as
hydrate dedup.

---

## Walk

PWA and Cab side by side, same Google, deployed origin.

1. Ask something that makes Kit call a tool. Both drafts stay on
   screen through the tool call; the chip stays; the `reply` replaces
   the draft. No blank.
2. Ask again; while Kit is streaming, leave Cab and come back after
   ~15 s (forces a sweep on resume). The draft is on screen when you
   return — either it never left (Cab fix) or the connect flush
   brought it back (Worker fix).
3. Same, but hide and show the PWA tab mid-answer. Draft comes back
   on redial.
4. Airplane mode for 3 s mid-answer, off again. Cab header drops and
   returns; the draft is there after reconnect.
5. Long answer (> 60 s of streaming). Header never shows "socket
   down"; if it does, it is the pong item.

## Watch

- Additive only. No new `kind`, no new required field. A re-sent
  `draft` on connect is a frame every mouth already paints.
- Do not replay drafts as part of the transcript or with
  `replay: true` — Cab's `shouldSpeak` and hydrate do not know
  drafts, and a stored draft would land in the thread cache.
- Do not "fix" (1) by having the Worker keep phone sockets alive
  longer; the sweep is deliberate, and `cancel()` on the retired
  socket is how it stays gapless on the phone.
- The chip and the draft are separate: `typing` TTL is a Cab
  constant; draft persistence is this doc.
