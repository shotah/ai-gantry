# Emoji reactions

Both directions: the agent reacting to the human's message, and the human
reacting to the agent's. The crane (`ai-gantry`) half is built and pinned
by tests; the Worker, PWA, and Cab halves are named at the end.

## Where it stands

| | Telegram | Pendant (PWA / Cab) | stdio / Discord / Slack |
| --- | --- | --- | --- |
| Human reacts to the agent | `message_reaction` updates settle 3 s, then `[reaction] 👍 on: <agent text>` is a turn; kernel triage decides whether the model is called. | Crane side ready: `react` frame in → settle → same turn. Worker / PWA / Cab do not send it yet. | — |
| Agent reacts to the human | `SetMessageReaction` on their message. | Crane side ready: `react` frame out on their inbound frame id. Nothing paints it yet. | Text fallback (`👍` as the reply). |

## The rule

A reaction is a reply. It is the acknowledgment the persona already bans
as text ("never a bare 'got it' / 'yes boss'") — allowed because it costs
the human nothing to read and costs the model five tokens to write.

- **Agent → human.** When the honest reply is an acknowledgment — noted,
  thanks back, nice, will do — react and write nothing. React *and* write
  when there is something to add: the wake that got set, the one question.
  Never as a substitute for a tool call or an answer. Not on `[input]
  spoken` turns; there is nothing to see in a car.
- **Human → agent.** Their 👍 on your message is not a question. It costs a
  model call only when it might change something: you were waiting on
  them, or the reaction is negative. Everything else is recorded and
  stays silent.

## Agent → human: the `[react …]` token

Same family as `[wait]` / `[nowait]` / `[silent]`: an own-line (or
trailing) reply token, not a tool.

```text
[react 👍]
```

```text
Set a 6pm wake to ask how it went.
[react 👍]
```

**Kernel.**

- `cron.StripWaitTokens` / `StripWaitTokensLive` also strip `[react …]`
  (complete, and a trailing partial while streaming), so every existing
  strip site — kernel `Handle`, Telegram and pendant `editStream` — already
  hides it. No new plumbing for the *text*.
- `cron.ReactEmoji(reply) string` returns the emoji. `Handle` puts it on a
  `channel.ReactionSink` attached to ctx by the mouth (same shape as
  `AttachPhotoSink`; mouths that cannot react do not attach one).
- History stores the raw reply, as it does `[wait]`: the model later sees
  `[react 👍]` as its own turn, not an empty message.
- No sink (stdio, Discord, Slack; a pendant frame without an `id`), an
  `[input] spoken` turn, or an emoji off the palette → the emoji goes out
  *as* the text when there is none (`👍`), and is dropped when there is. A
  "thanks!" never gets silence.

**Persona.** One kernel-owned section beside `## Follow-up` (`persona`
package, so it is written once, not per persona):

```markdown
## Reactions (`[react 👍]`)

- **Not a tool.** When the honest reply is an acknowledgment — noted,
  thanks back, nice, will do — put `[react <emoji>]` on its own line and
  nothing else. They see the emoji on their message, never the token.
- React and write only when there is something to add: `[react 👍]` plus
  the one line that matters.
- Not on `[input] spoken` turns. Not instead of a tool call or an answer.
- `[reaction] 👍 on: …` as *their* turn is them reacting to you. On a
  question you asked, 👍 / ❤️ is yes and 👎 is no — act on it, do not ask
  again. A 👎 elsewhere: fix or offer to, one line. Otherwise `[silent]`.
```

The exact text is `persona.ReactSection`; the palette line is built from
`channel.Palette` so the model, the mouths, and the picker share one list.

Palette the model is told about: `👍 👎 ❤️ 🔥 🤣 😢 🤔 🙏 👀 🎉 💯 👏`. It
can say no, cry, or shrug — a reaction that can only nod is a yes-man.
Every entry is in the standard set Telegram bots may set (✅ and 😂 are
not; 🤣 is), and it is what the phone picker shows. Telegram wants `❤`
without the variation selector; that mouth normalizes.

**Mouths.**

| Mouth | Delivery | Needs |
| --- | --- | --- |
| Telegram | `SetMessageReaction` on the human's `MessageID` (bare code point: `❤`, not `❤️`), after `handle` returns. Reaction-sourced turns have no human message: no sink. API rejects the emoji → text if the text is empty, else drop. | — |
| Pendant | Outbound frame `{"kind":"react","user_id":…,"id":<human's inbound frame id>,"text":"👍"}`. Existing fields, one new `kind`. Rides the `reply` path (dial fallback on a dead socket). Inbound frame without `id` → no sink → text fallback. | Worker fans `react` to phones and stores it on the transcript entry so hydrate paints it. PWA / Cab paint a chip on the human bubble. Old APK must ignore unknown `kind`. |
| stdio / Discord / Slack | Text fallback only. | — |

## Human → agent

**Pendant inbound.** `{"kind":"react","user_id":…,"id":<agent frame
id>,"text":"👍"}`. Empty `text` = cleared.

For the crane to say *what* was reacted to, it must know its own frame
ids: `reply` frames get a crane-stamped `id` (as `push` already does — the
Worker keeps a supplied id) and the channel keeps a small recent ring of
`id → text`, the pendant twin of Telegram's `outboundIndex`; `push` ids
are kept too (👍 on a morning cron is the common case). Unknown id →
`[reaction] 👍 on: (unknown message)`; still handled.

**Settle.** `channel.Settler` (3 s quiet, latest set wins, empty set
cancels), keyed per user + message. Telegram and pendant both use it. One
implementation, two mouths.

**Kernel triage — the cost rule.** On a `[reaction]` turn, before any
model call (`agent.triageReaction`):

| Session | Emoji | Action |
| --- | --- | --- |
| `waiting_for_reply=false` | all positive / neutral (`👍 ❤️ 🔥 😂 🙏 👏 💯 ✅ 🎉 😍 🤣 👀`) | **No model call.** Append the `[reaction]` user line and a `[silent]` assistant line to history (keeps alternation; reads as what the model would have said). Log `reaction silent skip` and a zero-cost `turn perf` (`source=reaction outcome=silent iterations=0`). |
| `waiting_for_reply=true` | any | Model turn. Their reaction is their reply: the wait clears and the follow-up pokes cancel first, as for a text reply. The persona line says 👍 / ❤️ = yes, 👎 = no. |
| any | any negative / uncertain (`👎 ❓ 😕 🤔 😡 😢`) or unknown (custom emoji) | Model turn. |

The alternative — every reaction to the model, persona steering it to
`[silent]` — is simpler and costs ~15k tokens per 👍. Declined.

Debounce is the settle above; there is no second "wait for a follow-up
text" timer. A text that follows a reaction sees the reaction in history
on its own turn.

## Order of work

Each step is additive: an old mouth sees only frames it already knows.

1. **Crane** (this checkout) — done. Token + strip + `ReactEmoji`;
   `ReactionSink`; kernel triage; persona section; Telegram
   `SetMessageReaction`; pendant `react` out (reply ids + `channel.Recent`)
   and in (`channel.Settler`); text fallback for the rest; eval fixtures.
   `frontends.md` wording for the Worker team is the two frame shapes above.
2. **Worker** (`gantry-pendant/worker/mailbox.ts`). Fan `react` both
   directions (through `fanOut`, off the rate bucket like `typing`); store
   reactions on the transcript entry; keep crane-stamped `reply` ids;
   `frontends.md` → Reactions.
3. **PWA** (`gantry-pendant` app). Paint the chip; long-press → picker
   (the palette) → send `react`; hydrate paints stored reactions.
4. **Cab** (`gantry-cab`). Same as PWA; `ignoredKind` for unknown kinds
   first, so a phone on the old APK is not confused by the first `react`.

## Eval

The harness attaches a `ReactionSink`, so a fixture sees the emoji where a
mouth would (`reaction=`), not in the text. Expectations: `react` (an
emoji, or `*`), `react_only` (emoji and no text), `no_model_call` (the
kernel settled it; rounds must be 0); fixture field `waiting` arms the wait
before the turn.

- `05_thanks_sounds_good`: a tool, a next question, or `[react 👍]` alone —
  never a bare "got it" in text.
- `12_react_to_thanks`: the thing is done, they say thanks → `react_only`.
- `13_reaction_idle_thumbs_up`: their 👍 on a done message → `no_model_call`,
  silent.
- `14_reaction_waiting_thumbs_up`: `waiting: true`, their 👍 on "block
  Thursday 7–8?" → the event is created, no question, wait cleared.
- `15_reaction_thumbs_down`: their 👎 on "Blocked Thursday 7–8 for the
  gym." → not silent; the event moved or deleted, or one question. (The
  scenario is a calendar block, not a wake, so the world state lives in a
  canned tool result: a fixture whose history says "wake set" while
  `[wakes]` is empty reads to the model as "you didn't", and it fixes that
  instead.)

12, 14 and 15 are live-model fixtures (`make integration-test`); 13 and the
plumbing of all four run on the scripted completer in `go test`.

## Decided (2026-09-17)

1. Palette: the fixed list above, negatives included.
2. Triage: kernel short-circuit for idle positive reactions; waiting or
   negative goes to the model.
3. `thanks_sounds_good`: `[react 👍]` alone passes.
4. Wire: `react` kind with `user_id` / `id` / `text`; crane stamps `reply`
   ids. Crane builds first; the shape goes to Worker, PWA, Cab as-is.
