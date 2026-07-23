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
| `/status` | Uptime, model, history size (estimated tokens), tool count |

Use **`/new`** when LOCAL_AGENT dumps huge JSON/transcripts, loops on the same tool error, or
ignores a clear ask. That is usually a poisoned session, not a broken deploy —
reset and ask one concrete thing again.

---

## Session bounds

gantry keeps the prompt bounded with env knobs (defaults are sane; all in
[ai-gantry §5.1](https://github.com/shotah/ai-gantry#51-environment-variables)):

- `HISTORY_MAX_MESSAGES=200` — hard message cap
- `HISTORY_MAX_TOKENS=128000` — estimated (chars/4); oldest turns drop first
- `TOOL_RESULT_MAX_CHARS=16000` — trims huge single tool results (Gmail dumps)
- Tool results older than the last 4 turns collapse to a one-line stub
- Trimmed turns fold into a rolling per-session **summary** via the same LLM

Gemini 3.5's ~1M window leaves headroom, but fat tool results still make
answers worse without these caps. `/new` remains the hard **session** reset.

Streaming replies (Telegram edit-in-place) are opt-in: `STREAM_REPLIES=true`.

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
