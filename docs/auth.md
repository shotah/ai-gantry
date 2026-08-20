# Auth from chat (remote OAuth, no inbound ports)

Headless boxes cannot complete the usual laptop OAuth dance
(`gantry auth google` → localhost callback → copy tokens). Chat `/auth` fixes
that with **zero inbound ports**.

Full design notes: [slash_commands_todo.md](../slash_commands_todo.md) § `/auth`.

---

## Two flows

| Flow | Servers | What you do |
| --- | --- | --- |
| **Auth-code paste + PKCE** | `google`, `strava`, `ghealth` | Open URL → approve → copy `code` from catch page → paste into chat |
| **Device flow** | `youtube` | Open URL → enter the short code → `/auth youtube wait` |
| **MFA paste** | `garmin` | Creds in `.env` → `/auth garmin` → email code → `/auth garmin <code>` |

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

A **Desktop** OAuth client (what laptop `gantry auth google` uses) only allows
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

YouTube stays on a **TV / Limited Input** client (device flow).

**Google Health** already uses a **Web** client — add the catch URI alongside
`http://127.0.0.1:4101/oauth2callback` on the same client (same pattern as
Workspace chat `/auth`).

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
   `127.0.0.1`, so laptop `gantry auth strava` still works with that domain set.

Chat `/auth strava` is implemented but **not fully smoke-tested** end-to-end
yet — confirm the authorize URL lands on the catch page and
`/auth strava <code>` writes tokens before relying on it in prod.

Localhost interactive auth (`gantry auth <server>`) is
**unchanged** and still uses `http://localhost:…`.

---

## Chat commands

```text
/auth                       # list auth-capable servers
/auth google                # print authorize URL (holds PKCE verifier ~10 min)
/auth google <code>         # exchange pasted code → tokens on disk
/auth strava                # same PKCE paste flow
/auth strava <code>
/auth ghealth               # same PKCE paste flow
/auth ghealth <code>
/auth youtube               # device: URL + user_code
/auth youtube wait          # finish device poll → tokens
/auth garmin                # start login (GARMIN_EMAIL/PASSWORD); may ask for MFA
/auth garmin <code>         # paste MFA code from email → session.json
```

PKCE verifier/state (or device_code / Garmin MFA cookies) lives in a pending
file next to that MCP's tokens, not in the gantry process. Restarting
`/auth <server>` replaces it. Pending files expire (~10 min PKCE/MFA, ~15 min
device).

---

## MCP CLI contract (for authors)

Additive subcommands next to interactive `auth` / `login`:

| Chat | MCP argv (after `auth_args`) |
| --- | --- |
| `/auth google` | `url` |
| `/auth google <code>` | `exchange <code>` |
| `/auth strava` | `url` |
| `/auth strava <code>` | `exchange <code>` |
| `/auth ghealth` | `url` |
| `/auth ghealth <code>` | `exchange <code>` |
| `/auth youtube` | `--start` |
| `/auth youtube wait` | `--wait` |
| `/auth garmin` | `url` |
| `/auth garmin <code>` | `exchange <code>` |

Stdout shape (PKCE / MFA — forwarded into Telegram):

```text
open <url>   # or: garmin: MFA required (email) — check your email/app …
then paste the code: /auth <server> <code>
guide: https://github.com/shotah/ai-gantry/blob/main/docs/auth.md
```

### Garmin (MFA paste)

Put credentials on the box only (never in chat):

```env
GARMIN_EMAIL=you@example.com
GARMIN_PASSWORD=…
```

Then `/auth garmin` → if Garmin sends MFA, paste `/auth garmin <code>`.
Laptop TTY `gantry auth garmin` still works.
---

## Laptop path (still preferred when you have a browser nearby)

```bash
gantry auth google      # localhost:4100
gantry auth strava      # localhost:19876
gantry auth ghealth     # 127.0.0.1:4101
gantry auth youtube     # device flow in the terminal
gantry auth garmin      # interactive TTY
```

See [deploy-docker.md § MCP tool auth](deploy-docker.md#mcp-tool-auth-browser-oauth).

---

## Security notes

- Only `TELEGRAM_ALLOWED_USERS` can run `/auth`.
- PKCE: a code alone (in chat history) is useless without the verifier on disk.
- Codes are single-use and short-lived; never log them at info level.
- Never paste Garmin (or any) **passwords** into chat — only MFA codes after
  `/auth garmin`. Keep `GARMIN_EMAIL` / `GARMIN_PASSWORD` in `.env` on the box.
