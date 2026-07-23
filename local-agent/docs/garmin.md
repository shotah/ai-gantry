# Garmin Connect (sleep, weight, readiness)

Give LOCAL_AGENT your Garmin-native recovery data — sleep, Index scale weigh-ins, Body
Battery / HRV, training readiness — via the
[go-garmin](https://github.com/shotah/go-garmin) CLI’s built-in MCP server
(`garmin mcp`). A static Go binary is baked into the image (like
`strava-mcp`); gantry launches it over stdio.

**Keep Strava** for the activity feed if you want; Garmin fills the gaps Strava
never had. Climbing grades / falls / sends come from typed splits + split
summaries (already in go-garmin MCP).

Build source: [shotah/go-garmin](https://github.com/shotah/go-garmin) (DI auth
fork of [llehouerou/go-garmin](https://github.com/llehouerou/go-garmin)) · see
also [docs/strava.md](strava.md).

```mermaid
flowchart LR
  GN[gantry daemon] -->|MCP stdio| GM["garmin mcp"]
  GM -->|session HTTPS| GC[Garmin Connect]
  GM --- TOK[("secrets/garmin/session.json")]
```

Auth is lighter than Strava: **no API app, no client id/secret.** Login once
interactively (email / password / MFA); the CLI writes `session.json`. Runtime
only needs that file + `HOME` so `os.UserConfigDir()` resolves to the mount.

Session path (go-garmin `session.go`):

```text
$XDG_CONFIG_HOME/garmin/session.json
# with HOME=/data in compose →
/data/.config/garmin/session.json
```

> Upstream README mentions `garmin login -email=… -password=…`, but current
> `login.go` is **interactive prompts only** (TTY). Use `make garmin-auth`.

---

## What LOCAL_AGENT can do

Tools reach the model prefixed as `garmin__…` (no approval gates in gantry —
listed in `mcp.toml` = granted):

| Ask | Tool |
|---|---|
| "How did I sleep last night?" | `get_sleep` |
| "What's my weight trend?" | `get_weight` |
| "Am I recovered enough to train?" | `get_body_battery`, `get_hrv`, `get_training_readiness` |
| "What did I do this week?" | `list_activities`, `get_activity` |

---

## 1. Optional `.env` pin

**No Garmin email/password in `.env`.** The image downloads the
[shotah/go-garmin](https://github.com/shotah/go-garmin/releases) release
binary (default `v0.1.2`). Override only to bump:

```env
# GARMIN_MCP_VERSION=v0.1.2
```

---

## 2. Authorize once (`make garmin-auth`)

```bash
make garmin-auth
```

That builds the image if needed, then:

```bash
docker compose run --rm --build -it --entrypoint garmin gantry login
```

1. Enter Garmin Connect **email**, **password**, and **MFA** if prompted.
2. On success: `Login successful.` and `secrets/garmin/session.json` on the host
   (mounted at `/data/.config/garmin`).

No published ports (unlike Strava’s OAuth callback). Re-run if the session
expires — `make garmin-auth` deletes any existing `session.json` first so a
stale “already logged in” file doesn’t block refresh.

If `.env` has `DEPLOY_HOST`, `make garmin-auth` also runs **`make garmin-sync`**
and pushes `session.json` to the server. `remote-deploy` does **not** copy
Garmin secrets (so a stale laptop file can’t clobber a good server session).

Push anytime without re-login:

```bash
make garmin-sync
```

---

## 3. Deploy / restart

```bash
make build           # bakes garmin into the image
make up              # or make remote-deploy
make garmin-sync     # only when you intentionally want the server to get this session
```

---

## Config wiring

`mcp.toml` already has (listed = granted; no bundles or approval lists):

```toml
[[server]]
name    = "garmin"
command = "garmin"
args    = ["mcp", "--tool-tier", "core"]
```

`--tool-tier core` publishes ~10 recovery/coaching tools (sleep, weight, body
battery, HRV, readiness, activities + climbing splits). Use `extended` or
`complete` if you need more — see [go-garmin MCP docs](https://github.com/shotah/go-garmin#narrowing-the-tool-surface).

Compose mounts:

```yaml
- ./secrets/garmin:/data/.config/garmin
```

`HOME=/data` is already set, so the CLI finds the session with no
extra env vars.

---

## Smoke tests

```bash
make build
docker compose run --rm --entrypoint garmin gantry --help

# After garmin-auth:
docker compose run --rm --entrypoint garmin gantry sleep
docker compose run --rm --entrypoint garmin gantry weight daily
```

Then ask LOCAL_AGENT over Telegram: “How did I sleep last night?” / “What’s my latest
scale weight?” / “How was my last climbing session — grades and falls?”

### Climbing API shape (what LOCAL_AGENT should use)

| Need | Tool | Fields |
|---|---|---|
| Session falls / sends / max grade | `list_activities` or `get_activity_split_summaries` | `numFalls`, `numClimbSends`, `numClimbsCompleted`, `maxClimbGrade` / `maxGradeValue` |
| Per-route grades + completed vs attempted | `get_activity_typed_splits` | `type`=`CLIMB_ACTIVE`, `status`=`CLIMB_COMPLETED`\|`CLIMB_ATTEMPTED`, `gradeValue` (`VERMIN`/`YDS`/`FONT`) |

Watch “falls” ≈ `numFalls` on the `CLIMB_ACTIVE` split summary (not a separate
endpoint). Bouldering often shows attempts via `CLIMB_ATTEMPTED` status instead.

---

## Troubleshooting

| Symptom | Likely fix |
|---|---|
| LOCAL_AGENT doesn’t see Garmin tools | Check the `[[server]]` entry in `mcp.toml`; rebuild so `garmin` is in the image |
| Boot fails with `mcp: boot server "garmin"` | `make build` / `make remote-deploy`; check `make logs` for the tool's stderr |
| `not logged in` | `make garmin-auth` (auto `garmin-sync` if `DEPLOY_HOST` set), or `make garmin-sync` |
| Auth / 401 after weeks | Session expired — re-run `make garmin-auth` (clears stale `session.json` first) |
| Rate limited (429) | Unofficial Connect API — ask for summaries, don’t poll |

### `OAuth2 exchange failed: 401` on `make garmin-auth`

**Your password is fine.** If you saw `Email:` / `Password:` / `MFA Code:` and then:

```text
Error: failed to exchange for OAuth2 token: OAuth2 exchange failed: 401 Unauthorized
```

that was the **pre-fix** go-garmin path (`…/oauth-service/oauth/exchange/user/2.0`).
LOCAL_AGENT now builds [shotah/go-garmin](https://github.com/shotah/go-garmin) with
mobile SSO + **DI** tokens (`diauth…/di-oauth2-service/oauth/token`), same idea as
`garminconnect` ≥ 0.3.

**If login still fails after rebuilding the image:**

1. **Stop retrying** for a bit — failed SSO can trigger account+client **429**
   blocks that last hours.
2. Cloudflare may still block plain Go TLS (Python uses `curl_cffi`). Note the
   exact status (403/429/other) before another attempt.
3. Keep Strava for workouts until auth sticks once (`session.json` then refreshes).

---

## Auth flow (vs Strava)

| | Strava | Garmin (go-garmin) |
|---|---|---|
| App registration | Strava API app + client id/secret in `.env` | None |
| Secrets in `.env` | `STRAVA_CLIENT_ID`, `STRAVA_CLIENT_SECRET` | **None** (optional `GARMIN_MCP_VERSION`) |
| One-shot auth | Browser OAuth + port `19876` | Interactive `garmin login` (TTY) |
| Persisted artifact | `secrets/strava/tokens.json` | `secrets/garmin/session.json` |
| Make target | `make strava-auth` | `make garmin-auth` |
| Runtime env | client id/secret + `STRAVA_TOKEN_PATH` | mount + `HOME` only |

---

## Follow-ups

- [x] Climbing typed-splits / grades / falls (go-garmin MCP)
- [ ] Decide whether to drop Strava once Garmin activity coverage feels enough

---

## Risks

Unofficial Connect API (can break), MFA/session expiry, young upstream (we pin
`GARMIN_MCP_VERSION`). Accept those or stay Strava-only for activities.
