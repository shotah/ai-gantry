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

Forks may keep using this catch page (public, no secrets) or host their own
Pages copy and set `*_OAUTH_REDIRECT_URI` + the matching OAuth client URI.

Register the same URI on each OAuth client (alongside localhost for the
laptop flow):

| Provider | Env override | Default |
| --- | --- | --- |
| Google Workspace | `GOOGLE_OAUTH_REDIRECT_URI` | catch page above |
| Strava | `STRAVA_OAUTH_REDIRECT_URI` | catch page above |
| Google Health | `GOOGLE_HEALTH_OAUTH_REDIRECT_URI` | catch page above |

### Google: you need a **Web application** client

A **Desktop** OAuth client (what laptop `make google-auth` uses) only allows
`http://localhost:…` redirects. Chat `/auth` sends users to the GitHub Pages
catch URI above — Google rejects that on Desktop (`redirect_uri_mismatch` /
“invalid request”).

In [Google Cloud Console → Credentials](https://console.cloud.google.com/apis/credentials):

1. **Create OAuth client → Web application** (do not try to flip an existing Desktop client).
2. Authorized redirect URIs — add **exactly** (trailing slash matters):
   ```text
   https://shotah.github.io/ai-gantry/oauth-catch/
   ```
   Optional on the same Web client if you want one pair of secrets for both flows:
   ```text
   http://localhost:4100/oauth2callback
   ```
3. Put that Web client’s ID/secret in `GOOGLE_OAUTH_CLIENT_ID` /
   `GOOGLE_OAUTH_CLIENT_SECRET` on the box, then regenerate env / restart.
4. Keep the Desktop client for laptop-only auth if you prefer separate secrets.

YouTube stays on a **TV / Limited Input** client (device flow). Google Health
docs already call for a Web client — same catch URI as above.

Scopes for Workspace chat auth must include `openid` +
`userinfo.email` (current `google-mcp` defaults) or userinfo returns 401 after
a successful code paste.

### Strava: Authorization Callback Domain

Strava’s app settings take **one domain** (not a path, not a list —
`localhost; shotah.github.io` / commas do not work).

For chat `/auth`, set:

1. [Strava API settings](https://www.strava.com/settings/api) →
   **Authorization Callback Domain** = `shotah.github.io`
   (or your fork’s Pages host).
2. Leave localhost alone — Strava always whitelists `localhost` /
   `127.0.0.1`, so laptop `make strava-auth` still works with that domain set.

Chat `/auth strava` is implemented but **not fully smoke-tested** end-to-end
yet — confirm the authorize URL lands on the catch page and
`/auth strava <code>` writes tokens before relying on it in prod.

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
