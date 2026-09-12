# Channels

One `CHANNEL` per process. All mouths are outbound-only + allowlist (empty
list fails boot). Default is Telegram. Discord, Slack, and
[pendant](https://github.com/shotah/gantry-pendant) are opt-in.
[Cab](https://github.com/shotah/gantry-cab) is another mouth on that same
mailbox, not a second channel. Stdio is the local REPL (`make run`).
Want two mouths? Two compose services. Family diagram:
[ecosystem.md](ecosystem.md).

Env table: [design.md](design.md#environment-variables). Allowlist is the
gate: [design.md](design.md#security).

---

## Discord (Gateway)

Outbound WebSocket. DMs only. No inbound ports.

1. [Discord Developer Portal](https://discord.com/developers/applications) →
   New Application → **Bot** → Reset Token.
2. **Bot → Privileged Gateway Intents:** enable **Message Content Intent**.
3. Invite the bot (OAuth2: scope `bot`). Open a **DM**.
4. Developer Mode → right-click your avatar → **Copy User ID**.

```bash
CHANNEL=discord
DISCORD_BOT_TOKEN=...
DISCORD_ALLOWED_USERS=123456789012345678   # snowflakes
```

Streaming default on. Image attachments → vision; reply images as embeds.
Cron push DMs every allowlisted user (CHANNEL is the destination).

---

## Slack (Socket Mode)

Outbound WebSocket. HTTP Events API (public Request URL) is a non-goal — it
would open a port.

1. [api.slack.com/apps](https://api.slack.com/apps) → Create From scratch.
2. Enable **Socket Mode**. App-level token (`xapp-…`, scope `connections:write`)
   → `SLACK_APP_TOKEN`.
3. Bot scopes: `chat:write`, `im:history`, `im:read`, `im:write`,
   `files:read`, `files:write`, `app_mentions:read`. Add
   `channels:history` / `groups:history` only if you want channel mentions.
4. Event Subscriptions (no Request URL): `message.im`, `app_mention`.
5. Install → Bot User OAuth Token (`xoxb-…`) → `SLACK_BOT_TOKEN`.
6. Profile → Copy member ID → allowlist.

```bash
CHANNEL=slack
SLACK_BOT_TOKEN=xoxb-...
SLACK_APP_TOKEN=xapp-...
SLACK_ALLOWED_USERS=U01234567
```

DMs stay flat; channel `@mention` replies land in a thread. Both tokens are
required (`xapp-` is not the bot token).

---

## Pendant (gantry-pendant Worker)

Outbound WSS to the [gantry-pendant](https://github.com/shotah/gantry-pendant)
mailbox. The crane dials; nothing listens. Setup in that repo’s `docs/setup.md`.
[Cab](https://github.com/shotah/gantry-cab) is another client on this mailbox
(Android Auto). The crane still uses `CHANNEL=pendant`.

```bash
CHANNEL=pendant
PENDANT_MAILBOX_URL=wss://gantry-pendant.<account>.workers.dev/ws/kit
PENDANT_BEARER=...
# Google sub, sub:email, or email
PENDANT_ALLOWED_USERS=118212345678901234567:ada@example.com, bob@example.com
```

Grant `image` and `pendant` in `mcp.toml` and set `IMAGE_OUTPUT_DIR` (both
children inherit it) so Kit can draw a face or wallpaper and wear it via
`source_path`. Chat photos still go on the mailbox reply; face and backdrop
are HTTP blobs, not bubbles.

Recreate the container after env changes (restart keeps a ghost allowlist).

| Entry | Parse |
| --- | --- |
| `118212345678901234567` | Google `sub` (digits) |
| `118212345678901234567:ada@example.com` | `sub` left of the first `:`, email right (lowercased) |
| `ada@example.com` | email only (lowercased) — alias until the `sub` is known |

The crane admits a frame when `user_id` **or** `email` is on the list. The
agent conversation is always `gantry` — email matches for admit, it never
keys memory, cron, or history. On dial the crane writes `cmds` then `allow`.
Email-only rows learn the Google `sub` on first inbound (or silent pin) so
Push can target that phone; until then Push broadcasts to `role:phone`.
Console may write emails into `PENDANT_ALLOWED_USERS`
([gantree-contract.md](gantree-contract.md)).
