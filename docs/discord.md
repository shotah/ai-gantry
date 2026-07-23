# Discord channel

Outbound-only Discord Gateway (WebSocket). Same security model as Telegram:
allowlist, no pairing, **no inbound ports**.

## Setup

1. [Discord Developer Portal](https://discord.com/developers/applications) →
   New Application → **Bot** → Reset Token → copy token.
2. Under **Bot → Privileged Gateway Intents**, enable **Message Content Intent**
   (required to read DM text).
3. Invite the bot to a server you share with yourself (OAuth2 URL Generator:
   scopes `bot`, permission optional). Then open a **DM** with the bot.
4. Enable Discord **Developer Mode** (User Settings → Advanced) → right-click
   your avatar → **Copy User ID** (snowflake).
5. Env:

```bash
CHANNEL=discord
DISCORD_BOT_TOKEN=...
DISCORD_ALLOWED_USERS=123456789012345678   # comma-separated snowflakes
# LLM_* as usual
```

6. `gantry run` (or compose). Message the bot in DM: `hi`, `/status`, `/new`,
   `/tools`.

## Behaviour (v1)

| | |
| --- | --- |
| Scope | **DMs only** (guild messages ignored for now) |
| Auth | Allowlist by user snowflake — others are logged and dropped |
| Commands | Same text commands as Telegram (`/new`, `/status`, `/tools`) |
| Cron push | DMs allowlisted users via stored DM channel id |
| Streaming | Not yet (full reply per message; edits are phase 2) |
| Attachments | Not yet (phase 2) |

## Notes

- Default `CHANNEL` remains `telegram`. Discord is opt-in.
- One channel per container — want Telegram *and* Discord? Two compose services.
- Bot must be able to receive your DMs (shared server + open DM, or prior DM).
