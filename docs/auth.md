# Auth from chat (remote OAuth, no inbound ports)

Headless boxes cannot complete the usual laptop OAuth dance
(`make google-auth` → localhost callback → scp tokens). Chat `/auth` fixes
that with **zero inbound ports**.

Full design notes: [slash_commands_todo.md](../slash_commands_todo.md) § `/auth`.

---

## Two flows

| Flow | Servers | What you do |
| --- | --- | --- |
| **Auth-code paste + PKCE** | `google`, `strava`, `ghealth` | Open URL → approve → copy `code` from catch page → paste into chat |
| **Device flow** | `youtube` | Open URL → enter the short code → `/auth youtube wait` |
| **TTY only** | `garmin` | Never in chat — `make garmin-auth` (email/password/MFA) |

---

## Catch page (PKCE redirect)

Authorize URLs use redirect:

```text
https://shotah.github.io/ai-gantry/oauth-catch/

```

That page is static HTML ([oauth-catch/index.html](oauth-catch/index.html)) —
it only displays `?code=` with a copy button. No server logic, no tokens.
CI publishes it to the repo `gh-pages` branch alongside the coverage badge
(so no separate Pages repo).

Register the same URI on each OAuth client (alongside localhost for the
laptop flow):

| Provider | Env override | Default |
| --- | --- | --- |
| Google Workspace | `GOOGLE_OAUTH_REDIRECT_URI` | catch page above |
| Strava | `STRAVA_OAUTH_REDIRECT_URI` | catch page above |
| Google Health | `GOOGLE_HEALTH_OAUTH_REDIRECT_URI` | catch page above |

Localhost interactive auth (`gantry auth <server>` / `make *-auth`) is
**unchanged** and still uses `http://localhost:…`.

---

## Chat commands

```text
/auth                     # list auth-capable servers
/auth strava              # print authorize URL (holds PKCE verifier ~10 min)
/auth strava <code>       # exchange pasted code → tokens on disk
/auth youtube             # device: URL + user_code
/auth youtube wait        # finish device poll → tokens
/auth garmin              # refused — use make garmin-auth
```

PKCE verifier/state (or device_code) lives in a pending file next to that
MCP's tokens, not in the gantry process. Restarting `/auth <server>` replaces
it. Pending files expire (~10 min PKCE, ~15 min device).

---

## MCP CLI contract (for authors)

Additive subcommands next to interactive `auth`:

| Chat | MCP argv (after `auth_args`) |
| --- | --- |
| `/auth google` | `url` |
| `/auth google <code>` | `exchange <code>` |
| `/auth youtube` | `--start` |
| `/auth youtube wait` | `--wait` |

Stdout shape (forwarded into Telegram):

```text
open <url>
then paste the code: /auth <server> <code>
guide: https://github.com/shotah/ai-gantry/blob/main/docs/auth.md
```

---

## Laptop path (still preferred when you have a browser nearby)

```bash
cd local-agent
make google-auth    # localhost:4100
make strava-auth    # localhost:19876
make ghealth-auth   # 127.0.0.1:4101
make youtube-auth   # device flow in the terminal
make garmin-auth    # interactive TTY
```

See [deploy-docker.md § MCP tool auth](deploy-docker.md#mcp-tool-auth-browser-oauth).

---

## Security notes

- Only `TELEGRAM_ALLOWED_USERS` can run `/auth`.
- PKCE: a code alone (in chat history) is useless without the verifier on disk.
- Codes are single-use and short-lived; never log them at info level.
- Never paste Garmin (or any) passwords into chat.
