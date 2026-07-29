# Telegram setup (ai-gantry)

Telegram is the **only** chat channel in this stack (by design — one persona,
one channel, one container). gantry **long-polls** the Bot API — no inbound
ports, no QR codes, no pairing flow.

---

## Steps

### 1. Create a bot

1. Open Telegram → talk to [@BotFather](https://t.me/BotFather).
2. Send `/newbot` and follow the prompts.
3. Copy the **bot token** (`123456:ABC...`).

### 2. Get your numeric user ID

1. Message [@userinfobot](https://t.me/userinfobot) (or similar).
2. Copy your **numeric ID** (e.g. `123456789`).
3. Open a chat with **your new bot** and send `/start` so it can reply later.

### 3. Configure this repo

In `.env`:

```env
GEMINI_API_KEY=AIza...
TELEGRAM_BOT_TOKEN=123456:ABC...
TELEGRAM_ALLOWED_USERS=123456789
```

Multiple users: comma-separated IDs — `111,222,333`.

That's the entire auth model: gantry answers listed IDs and ignores everyone
else. An **empty allowlist fails boot** (fail-fast) — there is no "allow all"
mode and no bind/pairing step.

Then:

```bash
make up
# or remote: make remote-deploy
make logs
```

### 4. Talk to the bot

Send a message in Telegram. Only IDs in `TELEGRAM_ALLOWED_USERS` are answered.
No approval step — if the allowlist is right, it just replies.

---

## In-chat commands

| Command | What it does |
| --- | --- |
| `/new` | Clear **this sender's** conversation history and start a fresh session |
| `/cancel` | Cancel the **in-flight** reply / tool loop for this chat (does not undo tools that already finished) |
| `/status` | Uptime, model, history + **schema** estimated tokens, tool count |
| `/tools` | Prefixed catalog + `schema_est_tokens` total and per-server breakdown |

Use **`/cancel`** when a turn is stuck on tools or you want to abort mid-reply, then send
the corrected ask. Use **`/new`** when LOCAL_AGENT dumps huge JSON/transcripts, loops on
the same tool error, or ignores a clear ask — that is usually a poisoned session, not a
broken deploy — reset and ask one concrete thing again.

### Multi-bubble asks (interrupt + coalesce + settle)

A single message starts work **immediately** — no settle delay. Coalescing only
engages once there is something to interrupt, so a follow-up bubble that lands
while Tim is still working causes gantry to:

1. **Interrupt** the in-flight turn (same plumbing as `/cancel`)
2. **Coalesce** the interrupted text with the new bubble(s)
3. **Settle** ~2s after the last message (`COALESCE_SETTLE_MS`, default `2000`; `0` disables)

Then run **one** joined turn. So "check Strava… wait, Garmin… nvm, calendar" fired
mid-turn becomes a single ask, while a lone question never pays the quiet window.
Tools that already finished are not undone. Cron and reaction synthetics skip this
path.

### "Hang on" line before the first token

With `STREAM_REPLIES=true` the bubble normally appears only once the model emits
something, so a slow prefill shows nothing but the typing dot. `SPINUP_NOTICE_MS`
(default `4000`) opens the bubble first:

- **Cold (first turn after restart)** — posts immediately; line picked at random
  from a small pool (model load / empty prompt cache).
- **Warm but slow** — posts after the threshold; another random pool line
  (often a prompt-cache miss).

Either line is a waiting indicator, not part of the answer: the reply replaces it
the moment Tim starts talking, and it never survives into the finished bubble
(tool traces still do). A fast turn never shows one at all. Set `0` to disable.

---

## Session bounds

gantry keeps the prompt bounded with env knobs (defaults are sane; all in
[ai-gantry §5.1](https://github.com/shotah/ai-gantry#51-environment-variables)):

- `HISTORY_MAX_MESSAGES=200` — hard message cap
- `HISTORY_MAX_TOKENS=128000` — estimated (chars/4); oldest turns drop first
- `TOOL_RESULT_MAX_CHARS=16000` — trims huge single tool results (Gmail dumps).
  Native/Ollama deploys default to `6000`: results are re-sent on every tool
  loop iteration, so the cap multiplies prefill cost
- Tool results older than the last 4 turns collapse to a one-line stub
- Trimmed turns fold into a rolling per-session **summary** via the same LLM

Gemini 3.5's ~1M window leaves headroom, but fat tool results still make
answers worse without these caps. `/new` remains the hard **session** reset.

Streaming replies (Telegram edit-in-place) are opt-in: `STREAM_REPLIES=true`.
When enabled, gantry caches the model text and flushes to Telegram about once
per second (so 429 flood-control does not stall the LLM or leave you with a
silent half-message). The final reply is always written on finish.

If the chat model emits chain-of-thought (Ollama/Qwen `reasoning` / `thinking`
fields), streaming shows it as **italics** above the answer (live edits would
reset Telegram’s expandable UI). The **final** message uses an expandable
italic blockquote. Set `LLM_REASONING_EFFORT=none` to disable thinking entirely
(the native default — thinking tokens are decoded before any tool fires).

Tool calls append a trace **inline** in the reply body (between prose chunks),
so a slow multi-tool turn shows motion without wiping earlier text:

```text
Here’s the math answer…

→ garmin__list_activities
✓ 1.2s · 4.1k chars

You rode 21mi.
```

CoT (when enabled) stays in the expandable italic block above; traces ride with
the conversation so a later tool failure cannot erase the earlier answer.
Timings also land in the journal (`model call` / `tool done` / `turn perf`) —
see [deploy-native.md](https://github.com/shotah/ai-gantry/blob/main/docs/deploy-native.md#latency-measure-before-tuning).

### Error reporting (ops alerts)

When you're remote and can't watch `journalctl`, tee slog failures into the
same Tim chat as a collapsed HTML box:

```env
TELEGRAM_ERROR_REPORTING=error   # off | error | warn
```

- Same DM you already talk in (`TELEGRAM_ALLOWED_USERS`)
- Shape: `🔴 gantry ERROR · <msg>` + expandable `<blockquote>` with attrs
- Dedupe: same message ≤ once per 5 minutes (suppressed count shown next time)
- Loop-safe: failures while sending the alert are dropped (never re-forwarded)
- Secrets in attr keys (`token`, `secret`, …) are redacted

Native deploy defaults this to `error`. Library / Docker default stays `off`.

### Reactions

Emoji reactions on bot messages (👍 ❤️ 😢 …) are treated as normal inbound
messages. gantry waits ~3s after the last change (so heart → thumbs-up settles
as one intent), then synthesizes a turn like
`[reaction] 👍 on: <clip of the bot message>` and runs the full agent loop.
No feature flag — if you don't want a reply, don't react. Clearing the reaction
during the wait cancels it.

### Rich inbound (tagged for the model)

Beyond text/photos, Telegram extras are turned into tagged lines (not separate
APIs):

| User sends | Agent sees |
| --- | --- |
| Location / venue | `[location] lat=… lon=…` / `[venue] Name — address (…)` |
| Contact | `[contact] Name, +phone` |
| Document | `[document] file.pdf (mime) N bytes` (metadata only) |
| Sticker | `[sticker] 🧗` (emoji when present) |
| Forward | `[forwarded from @user]` + body |
| Reply | `[reply to] <clip>` + body |
| Reaction | `[reaction] 👍 on: <clip>` (see above) |

Video, GIFs, and voice notes are ignored for now (no STT / no vision-video).

## Long-term memory

gantry's memory is **structured SQLite** (typed rows + FTS5 keyword search —
no embeddings) in `data/gantry.db`. See
[ai-gantry docs/memory.md](https://github.com/shotah/ai-gantry/blob/main/docs/memory.md).

- Storage is **deliberate**: LOCAL_AGENT calls `memory_store` for confirmed facts;
  nothing is auto-saved from chat. A background consolidation pass (default
  every 30 min) promotes episodes into durable facts/insights.
- `/new` clears the Telegram session only — long-term memory remains.
- Memory is inspectable and correctable: `make shell` then
  `sqlite3 gantry.db 'SELECT id, kind, subject, content FROM memory;'`,
  or ask LOCAL_AGENT to `memory_forget` the bad row.
- Persona files always outrank memory — identity lives in `persona/USER.md`.

---

## Security

- `TELEGRAM_ALLOWED_USERS` is required — boot fails without it, so there is no
  accidentally-open bot.
- Never commit `.env`.
- Bot token = full control of that bot; rotate via BotFather if leaked.
- No ports are opened by the container, ever — there is no gateway or
  dashboard to protect.
