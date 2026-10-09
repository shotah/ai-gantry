# Google Workspace auth: why `/auth google` warns and Pendant login does not

`/auth google` lands on **"Google hasn't verified this app"** (Advanced → Go
to shotah.github.io (unsafe)). Signing into the Pendant with the same Google
account shows a normal consent screen. Both OAuth clients are **Web
application** type in the same Cloud project, so the client type is not the
difference.

This doc explains what actually drives the warning and what the realistic
options are. Chat flow mechanics live in [auth.md](auth.md).

**Status (2026-10-07):** option 1 is done — branding verified and published
as **gantree** on `gantry.bldhosting.com`. The consent screen now shows the
name and logo; the sensitive-scope interstitial remains (options 3–5). What
is left on the box side is in [auth.md](auth.md#catch-page-pkce-redirect):
set the three `*_OAUTH_REDIRECT_URI` overrides and register the verified URI
on each client.

---

## What decides the warning

Google decides whether to show the unverified-app interstitial from three
things, none of which is the client type:

| Input | Where it lives | Pendant login | `/auth google` |
| --- | --- | --- | --- |
| **Scopes in the authorize URL** | Each request | `openid email profile` — all **non-sensitive** | Gmail, Drive, Calendar, Docs, Sheets, Tasks, Contacts — **sensitive + restricted** |
| **Publishing status** | Project → OAuth consent screen | shared | shared |
| **Verification status** | Project → Verification Center | shared | shared |

Non-sensitive scopes never trigger the interstitial, verified or not. That is
the whole reason Pendant looks "clean": Sign-in only asks who you are. The
moment any request carries a sensitive or restricted scope from an unverified
project, every user gets the warning, regardless of which client ID sent it.

The two clients share one consent screen (one project), so publishing status
and verification are the same for both. If the project were ever verified for
Workspace scopes, `/auth google` would get the clean screen too. Conversely,
nothing you change on the `pendant-oauth` or `tim the agent (chat)` client
rows will make the warning go away.

### Why it says "shotah.github.io" and not "tim the agent"

When branding is unverified, Google refuses to show the configured app name or
logo and falls back to the **redirect URI's domain**. The catch page is on
`https://shotah.github.io/ai-gantry/oauth-catch/`, so the consent and the
"granted access" notice both name `shotah.github.io`. Pendant's redirect is on
the Pendant Worker host, so it names that instead. Brand verification (below)
fixes the label; it does **not** remove the interstitial for sensitive
scopes.

### Scope classes in the `everyday` preset

`google-mcp --preset everyday` authorizes gmail, calendar, docs, sheets,
tasks, contacts, drive. Google classes them roughly as:

| Class | Scopes in play | Verification needed |
| --- | --- | --- |
| Non-sensitive | `openid`, `userinfo.email`, `userinfo.profile`, `drive.file` | None |
| Sensitive | Calendar, Tasks, Contacts (People), Docs, Sheets, `gmail.send`, `gmail.labels` | Brand + scope review (questionnaire, demo video) |
| Restricted | Gmail read/modify (`gmail.readonly`, `gmail.modify`, `mail.google.com`), full Drive (`drive`, `drive.readonly`) | Everything above **plus a CASA security assessment** (paid third-party, annual) |

Reading mail at all puts the project in the restricted tier. That is the
expensive line.

---

## Options

Ordered from zero effort to full product launch. The honest summary: there is
no OAuth plumbing change that gives Gmail-read the Pendant experience. Only
**Internal** (Workspace org) or **full verification** removes the screen.

### 1. Keep it, and make it look intentional (now, free)

This is Google's documented **personal use** exception: the developer and a
few people known personally click through. Friends sharing one project are
inside it, under the 100-grant cap.

- Leave the consent screen in **In production** (published), not Testing.
  See option 2 for why.
- Click **Advanced → Go to shotah.github.io (unsafe)** once per account.
  The refresh token on the box then lives until revoked.
- Do **brand verification** so the screen shows the agent's name and logo
  instead of `shotah.github.io`. Still an interstitial, but a named one, and
  the "granted access" notice names the agent too.

#### Why `shotah.github.io` cannot be the brand domain

Google's first verification run returned:

| Error | Cause | Fix |
| --- | --- | --- |
| Home page `…/gantree/` is not registered to you | `github.io` is a public-suffix domain; Google wants Search Console ownership of the *registrable* domain and GitHub owns it | Serve the site from a domain you own |
| Home page does not link the privacy policy | The gantree site had no link | The new home page footer links `privacy/` |
| Privacy policy URL unresponsive | Not published yet (CI had not run on `main`) | Merge; CI publishes it |
| Privacy policy insufficient content | Follows from the 404; policy also lacked retention / children / changes sections | Added |
| App name `tim_the_agent` not a brand | One persona, not a product | `gantree` |

The only domain you own is `bldhosting.com`, so the whole Pages site moves to
a subdomain of it. Forget the gantree Pages site for OAuth purposes.

#### Target layout

GitHub Pages custom domain **`gantry.bldhosting.com`** → this repo's
`gh-pages` branch. Every OAuth-facing URL lives here:

| URL | Source | Purpose |
| --- | --- | --- |
| `https://gantry.bldhosting.com/` | `docs/site/index.html` | **Home page** — the household (agent, pendant, cab, helm), what it does with Google, footer link to the policy |
| `https://gantry.bldhosting.com/privacy/` | `docs/site/privacy/index.html` | **Privacy policy** — agent scopes, Sign-in, Strava, Garmin, retention, deletion, Limited Use |
| `https://gantry.bldhosting.com/oauth-catch/` | `docs/oauth-catch/index.html` | **Redirect URI** for Google / Google Health / Strava chat auth |
| `https://gantry.bldhosting.com/logo.svg` | `assets/logo.svg` | Site logo (console upload uses `assets/logo.png`) |

`https://shotah.github.io/ai-gantry/…` keeps working as a 301 to the custom
domain once the CNAME is in place, but it must not appear in any OAuth
setting or `shotah.github.io` has to stay in Authorized domains and keeps
failing.

CI writes two things from **GitHub Actions repository variables**
(Settings → Secrets and variables → Actions → Variables):

| Variable | Effect |
| --- | --- |
| `PAGES_CUSTOM_DOMAIN` = `gantry.bldhosting.com` | Writes the `CNAME` file on `gh-pages` so a rebuild never drops the custom domain. Forks leave it unset. |
| `GOOGLE_SITE_VERIFICATION` | Fills the home page `<meta name="google-site-verification">`. Only needed if you verify by URL prefix instead of DNS (below). |

#### Steps

1. **DNS** at your `bldhosting.com` provider:
   - `gantry` → `CNAME` → `shotah.github.io.`
   - A `TXT` record at the apex for Search Console (step 3 gives the value).
2. **GitHub**: repo Settings → Pages → Custom domain `gantry.bldhosting.com`,
   tick **Enforce HTTPS** once the cert issues. Set the repo variable
   `PAGES_CUSTOM_DOMAIN` = `gantry.bldhosting.com`. Merge this branch to
   `main`; CI publishes the site with the `CNAME`.
3. **Search Console** → Add property → **Domain** → `bldhosting.com` →
   DNS TXT. Use the Google account that owns the Cloud project. A Domain
   property covers every subdomain, so this one verification also covers a
   future `pendant.bldhosting.com` for the Worker. (`GOOGLE_SITE_VERIFICATION`
   is only for the URL-prefix fallback; leave it unset if DNS works.)

   **The property must stay in Search Console.** "Not registered to you"
   means Google's OAuth side found no *current* verified owner of the domain
   among the project's owners/editors; the TXT record alone proves nothing.
   Google syncs this roughly daily, hence the "wait 24 hours" in the error.
   The old Cloud Console *APIs & Services → Domain verification* page is
   gone; Search Console is the only place this is managed now.
4. **OAuth client (Web, agent)** → Authorized redirect URIs: add
   `https://gantry.bldhosting.com/oauth-catch/` (trailing slash). Remove the
   `shotah.github.io` one once the box is switched.
5. **On the box** (`.env`, then restart):
   ```env
   GOOGLE_OAUTH_REDIRECT_URI=https://gantry.bldhosting.com/oauth-catch/
   GOOGLE_HEALTH_OAUTH_REDIRECT_URI=https://gantry.bldhosting.com/oauth-catch/
   STRAVA_OAUTH_REDIRECT_URI=https://gantry.bldhosting.com/oauth-catch/
   ```
   Strava API settings → Authorization Callback Domain =
   `gantry.bldhosting.com`. `google-mcp`'s compiled-in default still points at
   `shotah.github.io`; the env override wins, and changing the default is a
   `google-mcp` change.
6. **Google Auth Platform → Branding**:

   | Field | Value |
   | --- | --- |
   | App name | `gantree` |
   | User support email | your Gmail |
   | App logo | `assets/logo.png` (512×512, PNG; from `logo.svg` via `rsvg-convert -w 512 -h 512`) |
   | Application home page | `https://gantry.bldhosting.com/` |
   | Application privacy policy link | `https://gantry.bldhosting.com/privacy/` |
   | Terms of service | blank |
   | Authorized domains | `bldhosting.com` — and `christopherblodgett.workers.dev` only until the Pendant Worker moves to a `bldhosting.com` subdomain; delete `shotah.github.io` |

   `workers.dev` is also a public suffix, so `christopherblodgett.workers.dev`
   has the same ownership problem `github.io` had. Give the Worker a
   Cloudflare custom domain under `bldhosting.com` (gantry-pendant change),
   point the `pendant-oauth` client's redirect URI at it, and the list
   becomes just `bldhosting.com`.
7. Wait the 24 hours Google asks for after domain verification, then
   **Verify Branding** → **Publish branding** within 7 days.

One project = one consent screen: the name and logo land on Pendant / Cab /
Helm sign-in and on `/auth google` alike, which is why it is the household
brand and not an agent's persona.
Cost: nothing. Result: still an Advanced click, but the screen says who is
asking.

### 2. Do not "fix" it by switching to Testing

Testing mode looks tempting (add yourself as a test user, non-testers are
blocked) but it is worse for a headless crane:

- Test users still see a warning screen (a different one).
- **Refresh tokens expire after 7 days** when any requested scope is outside
  name/email/profile. `/auth google` would have to be redone weekly.
- 100 test users max.

Pendant would be unaffected (Sign-in scopes are exempt from the 7-day rule),
but `google-mcp` would break every week. Stay published.

### 3. Internal user type (clean screen, no verification) — needs Workspace

If the operator's account is in a **Google Workspace** (or Cloud Identity)
organization and the Cloud project is **owned by that org**, set the consent
screen user type to **Internal**. No interstitial, no verification, no user
cap, no 7-day tokens — for accounts in that domain.

Limits:

- The project in the screenshot is owned by a personal `@gmail.com`
  developer account. Internal is not offered to no-org projects; you would
  create (or already have) a Workspace tenant and migrate the project into
  it.
- Only accounts **in that Workspace domain** can authorize. Personal
  `@gmail.com` users (including the developer's own personal account) cannot.
- Pendant login would then also be domain-only for that project. Keep Pendant
  Sign-in in a separate, External project if it must stay open to gmail.com
  accounts (Google recommends separate projects anyway).

Best fit if the agent is for a household/business that is already on
Workspace.

### 4. Shrink to sensitive-only and verify (no CASA)

Drop every restricted scope so verification becomes a questionnaire + video
instead of a paid security assessment:

| Keep | Give up |
| --- | --- |
| Calendar, Tasks, Contacts, Docs, Sheets | Gmail **read** (search / get / threads / labels-modify) |
| `gmail.send` (send only) | Full Drive search — use `drive.file` (only files the app created or the user picked) |

Then do brand verification (option 1) plus **Data access → Add scopes** and
submit. Google asks for a scope justification per scope and a short screen
recording of the consent flow and the feature using each scope. Typically a
few business days to a few weeks; no cost.

Mechanically this is a `google-mcp` change (a preset or `--tools` set that
requests no restricted scope), not an `ai-gantry` change — `/auth google`
just forwards to `google-mcp auth url`. Decide whether a mail-blind assistant
is still the product before doing it.

### 5. Full verification including restricted scopes

What a real multi-user product does: option 4's paperwork plus a **CASA**
(Cloud Application Security Assessment) for restricted Gmail/Drive scopes,
re-done annually. Requires a third-party assessor and a public product with
a real privacy policy. Not justified for a bring-your-own-client agent.

### 6. Bring-your-own-project (the model the docs already assume)

[auth.md](auth.md) already tells each operator to create **their own** Web
client and put the ID/secret in `GOOGLE_OAUTH_CLIENT_ID/SECRET`. Lean into
it: every operator is the developer of their own project, so the personal-use
exception applies to them by design, and the 100-user cap and verification
never come into play. The interstitial is then an expected one-time step in
the setup doc, not a defect. Nothing to build; just say it plainly in the
Google section of [auth.md](auth.md) and in the `google-mcp` README.

---

## Recommendation

| Situation | Do |
| --- | --- |
| Just you, today | **Option 1 + 6**: stay published, brand-verify so it names the agent, document the click-through. |
| Operator has Workspace | **Option 3** for the agent project; keep Pendant Sign-in in an External project. |
| Want a clean screen for gmail.com users and can live without reading mail | **Option 4**. |
| Shipping to strangers with Gmail read | **Option 5**, and budget for CASA. |

Do not switch to Testing (option 2).

---

## Housekeeping while you are in the console

- The ⚠ on the `tim the agent (chat)` client row is worth hovering. Google
  now flags Web clients that have been **unused for ~6 months and are
  scheduled for deletion**, and clients with missing/invalid redirect URIs.
  If it is the former, a successful `/auth google` exchange clears it; if the
  latter, confirm `https://gantry.bldhosting.com/oauth-catch/` (trailing
  slash) is listed exactly.
- `Agent to …` (TV and Limited Input) is the YouTube device-flow client —
  unrelated to this warning.
- `gantry-helm` (iOS) and `gantry-cab` (Android) are Sign-in-only clients,
  same as Pendant, and will never show the interstitial.
