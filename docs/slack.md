# Slack channel (Socket Mode)

Outbound-only Slack via **Socket Mode** (WebSocket). Same security model as
Telegram/Discord: allowlist, no pairing, **no inbound ports**.

HTTP Events API (public Request URL) is a **non-goal** — it would require
opening a port.

## Setup

1. [api.slack.com/apps](https://api.slack.com/apps) → Create New App → From
   scratch.
2. **Socket Mode** → Enable Socket Mode.
3. **Basic Information → App-Level Tokens** → Generate token with scope
   `connections:write` → copy (`xapp-…`) → `SLACK_APP_TOKEN`.
4. **OAuth & Permissions** → Bot Token Scopes, add at least:
   - `chat:write`
   - `im:history`
   - `im:read`
   - `im:write`
   - `files:read` (vision: download inbound images)
   - `files:write` (outbound image uploads)
   - `app_mentions:read` (for `@bot` in channels)
   - `channels:history` / `groups:history` only if you want mentions in channels
5. **Event Subscriptions** → Enable Events (Socket Mode does not need a Request
   URL). Subscribe the **bot** to:
   - `message.im`
   - `app_mention`
6. Install app to workspace → copy **Bot User OAuth Token** (`xoxb-…`) →
   `SLACK_BOT_TOKEN`.
7. Find your Slack member ID (profile → ⋯ → Copy member ID) → allowlist.

```bash
CHANNEL=slack
SLACK_BOT_TOKEN=xoxb-...
SLACK_APP_TOKEN=xapp-...
SLACK_ALLOWED_USERS=U01234567
# LLM_* as usual
```

8. `gantry run`. DM the bot: `hi`, `/status`, `/new`, `/tools`. Or `@bot hello`
   in a channel the bot can see.

## Behaviour

| | |
| --- | --- |
| Transport | Socket Mode only (outbound WS) |
| Scope | DMs (`message.im`) + `@mention` in channels |
| Auth | Allowlist by Slack user id — others logged and dropped |
| Commands | Text `/new` `/status` `/tools` (same as Telegram) |
| Threads | Channel `@mention` replies land in a thread; DMs stay flat |
| Cron push | Posts to stored channel / opens DM for allowlisted user |
| Streaming | Opt-in `STREAM_REPLIES=true` — post placeholder, then `chat.update` |
| Files | Image shares → vision; reply markdown/`https` as image blocks; `data:` via file upload |

## Notes

- Default `CHANNEL` remains `telegram`. Slack is opt-in.
- One channel per container.
- App token (`xapp-`) is **not** the bot token — both are required.
