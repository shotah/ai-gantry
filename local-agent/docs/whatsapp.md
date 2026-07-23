# WhatsApp setup (legacy — ZeroClaw era)

> **Status: not applicable on ai-gantry.** This doc is kept for reference from
> the ZeroClaw days. gantry ships Telegram + stdio only — extra channels are an
> explicit non-goal (one persona, one channel, one container). If a friend
> needs WhatsApp, that would be a separate runtime, not a flag here.

Telegram stays the default channel. WhatsApp is **optional** when friends only
live on WhatsApp (DM or a small group).

Upstream:
[ZeroClaw WhatsApp channel](https://github.com/zeroclaw-labs/zeroclaw/blob/master/docs/book/src/channels/whatsapp.md)
· [channels overview](https://github.com/zeroclaw-labs/zeroclaw/blob/master/docs/book/src/channels/overview.md).

ZeroClaw supports **two backends** under the same `channels.whatsapp.*` family.
The mode is selected by which field you set — **do not set both** on one alias
(if you do, Cloud API wins for backward compatibility):

| Mode | Use when | Required selector | Public HTTPS? |
|---|---|---|---|
| **WhatsApp Web** | Link a normal WhatsApp account (QR / pair code) | `session_path` | **No** |
| **WhatsApp Cloud API** | Meta Business app + Business phone number | `phone_number_id` (+ tokens) | **Yes** (webhook / tunnel) |

---

## Pick for LOCAL_AGENT (decision)

Goal: LOCAL_AGENT is a **separate WhatsApp contact**, not “your personal phone typing.”
WhatsApp identity **is** the phone number.

| Path | Who LOCAL_AGENT appears as | Fits this stack? | Pick? |
|---|---|---|---|
| **A. Web + dedicated number** | LOCAL_AGENT’s SIM / eSIM / old phone | ✅ no inbound ports; peer allowlist | **Yes — recommended** |
| **B. Cloud API (Meta Business)** | Official WA Business number | ❌ needs public webhook + Meta app | Only if you want a verified business brand |
| ~~Link your personal WhatsApp~~ | **You** (LOCAL_AGENT speaks as you) | Easy, wrong identity | No |

### Recommendation

**Path A (Web mode + `mode = "personal"`).** Put WhatsApp on a spare **carrier**
line (see [Getting a real number](#getting-a-real-number-for-local-agent) below). Link
that device once; friends add **LOCAL_AGENT’s number**. Gate senders with
`peer_groups` + `dm_policy` / `group_policy` allowlists.

Skip Cloud API unless you accept tunnels and Meta Business setup. Same “no
ports” fight as [SMS](sms.md).

---

## Getting a real number for LOCAL_AGENT

WhatsApp needs a **mobile-network number that can receive SMS** for the one-time
verification code. That is what “real SIM” means here — not “plastic card,” not
“Google Voice,” not “Twilio long code.”

| Works for WhatsApp signup | Usually fails |
|---|---|
| Carrier prepaid / postpaid (T-Mobile, AT&T, Verizon, Fi, Mint, …) | Google Voice, TextNow, Skype |
| Carrier **eSIM** with calls + SMS | Twilio / Telnyx / most “API SMS” numbers |
| Physical nano-SIM from a real MVNO | Data-only travel eSIMs (no SMS inbox) |

After signup, WhatsApp itself runs over **data / Wi‑Fi**. The line mainly has to
(1) receive the verify SMS once, and (2) stay active enough that Meta doesn’t
kill the account. For LOCAL_AGENT + ZeroClaw Web mode you then **link that WhatsApp as
a linked device** to the server (QR) — the phone can sit on Wi‑Fi afterward.

### Proposed setups (cheapest → nicest)

#### 1. Cheap prepaid physical SIM on an old phone (best default)

Buy a **prepaid SIM** (Mint, Ultra Mobile, Tello, Visible, Walmart / grocery
MVNOs, etc.) on the **lowest talk/text plan** (~$5–15/mo, sometimes less). Put
it in a spare Android (drawer phone is fine).

1. Activate SIM → receive WhatsApp SMS on that number.
2. Install WhatsApp on that phone as **LOCAL_AGENT**.
3. On the server: enable Web mode → scan QR from that phone
   (**Linked devices**).
4. Leave the old phone plugged in on Wi‑Fi so the session stays healthy
   (optional but reduces random logouts).

No dual-SIM juggling on your daily driver. Number is clearly “LOCAL_AGENT’s.”

#### 2. Second line on **your** phone (eSIM + dual WhatsApp)

If your phone supports **dual SIM** (Pixel / modern Samsung / recent iPhone):

1. Add a second **carrier** eSIM or nano-SIM (same prepaid options as above).
2. Keep **your** number on personal WhatsApp.
3. Put LOCAL_AGENT’s number on a second WhatsApp surface:
   - **Android:** WhatsApp → Settings → profile dropdown → **Add account**, or
     use **WhatsApp Business** with “use a different number.”
   - **iPhone:** personal WhatsApp + **WhatsApp Business** (two apps).
4. Link **LOCAL_AGENT’s** account (not yours) to ZeroClaw via QR.

Set default calls/SMS/data so LOCAL_AGENT’s line isn’t burning your primary plan.

#### 3. Google Fi group line — yes, but it’s a full line

If you’re already on Fi: **adding a group member is a real cellular line**
(eSIM or SIM), with SMS — WhatsApp should accept it. It is **not** “order a
free extra number on my existing Fi SIM,” and it is **not** Google Voice.

Rough shape ([Fi plans](https://support.google.com/fi/answer/9462098),
[add a person](https://support.google.com/fi/answer/7131470)):

- Flexible: ~$15–20/mo per extra line for unlimited call/text, **plus shared
  data at $10/GB**.
- Unlimited tiers: often **+$25–40/mo** for the second line depending on plan.

**Do this if** you want one bill, easy eSIM on a Pixel, and don’t mind paying
Fi group pricing. **Skip if** LOCAL_AGENT only needs a verify SMS + occasional keep-
alive — a $5–10 prepaid plan is usually enough.

**Do not** use Google Voice / Fi “number forwarding tricks” as the WhatsApp
identity — Voice is VoIP-class and WhatsApp routinely blocks it.

#### 4. What *not* to do

| Idea | Why it fails |
|---|---|
| Twilio / Telnyx “SMS API” number | WhatsApp treats most as VoIP; signup fails |
| Google Voice | Same |
| Data-only travel eSIM | No SMS OTP inbox |
| Port your personal number to LOCAL_AGENT | You lose that number as *your* WhatsApp |

### Suggested pick for you

| If… | Do this |
|---|---|
| You have an old Android in a drawer | **Prepaid SIM (~$5–15/mo)** → LOCAL_AGENT’s WhatsApp → QR to ZeroClaw |
| No spare phone, dual-SIM phone | **Cheap carrier eSIM** + WhatsApp multi-account / Business |
| Already on Fi and want one bill / Pixel eSIM | **Add a Fi group line** for LOCAL_AGENT (~$15+/mo) — works, just pricey vs prepaid |
| Friends will use Discord instead | Skip WhatsApp entirely |

Then continue with the [Web mode recipe](#friend--group-recipe-web-mode--recommended) below.

### Still don’t want another line?

| Option | LOCAL_AGENT’s identity | Friend friction | Fits stack? |
|---|---|---|---|
| **Discord bot** | @LOCAL_AGENT | Low if friends already there | ✅ Best next step |
| **Slack** | Bot in a workspace | Invite-only crew | ✅ |
| **Stay on Telegram** | Already works | High if nobody uses it | ✅ Done |
| **SMS (Twilio / Telnyx)** | Rented number | Universal, clunky | ❌ webhook + 10DLC — [sms.md](sms.md) |

---

## How the two modes work (upstream)

### Web mode

- No Meta Business account.
- Needs a ZeroClaw build with the **`whatsapp-web`** feature and a **persistent**
  `session_path` (deleting it forces a fresh device link).
- First start: **QR** (default) or **pair-code** linking (`pair_phone` seeds
  pair-code; leave unset for QR).
- Bind the channel on the agent: `agents.main.channels` must include
  `"whatsapp.default"` (or your alias).
- Optional: `interrupt_on_new_message = true` cancels an in-flight reply when
  the same sender sends again (also applies to Cloud API).

### Cloud API mode

- Meta Business account, WhatsApp product, **phone number ID**, access token,
  verify token (and preferably `app_secret` for signature checks).
- Meta POSTs inbound traffic to your gateway. Configure ZeroClaw’s top-level
  `[tunnel]` (or your own reverse proxy).
- Callback URL is **per alias**:
  `GET`/`POST https://<public-host>/whatsapp/<alias>`
  e.g. `[channels.whatsapp.default]` → `/whatsapp/default`.
  Bare `/whatsapp` still works but is **deprecated** (first alias
  lexicographically; sets `X-Zeroclaw-Deprecation`).

---

## Friend + group recipe (Web mode — recommended)

### 1. Channel config (`mode = "personal"`)

Upstream default is `mode = "business"`, which **does not** apply the personal
DM/group policy split. For a peer-gated home assistant, use **`personal`**:

```toml
[channels.whatsapp.default]
enabled = true
session_path = "/zeroclaw-data/.zeroclaw/state/whatsapp-web/session.db"
# Optional: E.164 — seeds pair-code linking; omit for QR
# pair_phone = "15551234567"

mode = "personal"
dm_policy = "allowlist"       # allowlist | ignore | all
group_policy = "allowlist"    # allowlist | ignore | all
mention_only = true           # groups: only reply when @mentioned
self_chat_mode = false
# interrupt_on_new_message = true

# Optional: hear group chatter without spending tokens until @mentioned
# passive_group_context = true

# Optional: lock to specific groups by JID (DMs always bypass this)
# allowed_groups = ["120363012345678901@g.us", "120363098765432109"]
```

| Field | Effect (Web + `personal`) |
|---|---|
| `dm_policy` | Who may DM |
| `group_policy` | Whether groups are considered at all |
| `mention_only` | In groups, require @mention to start a turn |
| `self_chat_mode` | Allow/deny messaging yourself |
| `passive_group_context` | Store allowed unaddressed group lines as context **without** calling the model |
| `allowed_groups` | Drop groups whose JID isn’t listed (exact full JID or exact user part before `@`) |

Empty `allowed_groups` = all groups (still subject to policies / peers).
**DMs always bypass `allowed_groups`.**

Create the session dir once (deploy user ownership):

```bash
make remote-ssh CMD='mkdir -p config/state/whatsapp-web'
```

(`config/` mounts to `/zeroclaw-data/.zeroclaw` — adjust if you put the DB under
`data/` instead.)

### 2. Agent + peer allowlist (schema v3)

```toml
[agents.main]
channels = ["telegram.default", "whatsapp.default"]

[peer_groups.whatsapp_default]
channel = "whatsapp.default"
agents = ["main"]
# Your number + friend’s number (E.164)
external_peers = ["+15551110001", "+15551110002"]
```

Keep Telegram’s peer group so both channels work.

### 3. Link the device

```bash
make remote-deploy
make remote-logs
```

On LOCAL_AGENT’s phone: **WhatsApp → Linked devices → Link a device**, scan the QR
(or enter the pair code). Session lives at `session_path` — don’t delete it.

Then smoke-check (inside the container / via remote SSH):

```bash
zeroclaw channel doctor
# if needed:
zeroclaw channel start
```

> **Image note:** Web mode needs `whatsapp-web` in the binary. If logs say Web
> isn’t available with `session_path` set, check `ZEROCLAW_BASE` release notes
> or upstream — Cloud API may still work but needs a tunnel.

### 4. DM / group

1. Friend texts **LOCAL_AGENT’s** number; their E.164 must be in `external_peers`.
2. Group: you + friend + LOCAL_AGENT’s number. With `mention_only = true`, LOCAL_AGENT stays
   quiet until @mentioned.
3. Optional: set `allowed_groups` to that group’s `@g.us` JID from logs.
4. Optional: `passive_group_context = true` so LOCAL_AGENT can use prior group chatter
   as context when finally @mentioned (no model calls on every line).

---

## Cloud API (optional / heavier)

Only if you want Meta’s official Business Platform:

```toml
[channels.whatsapp.default]
enabled = true
# Do NOT also set session_path on this alias
access_token = "EAAB..."
phone_number_id = "123456789012345"
verify_token = "pick-a-long-random-string"
# app_secret = "..."   # recommended
```

1. Meta Developer app → WhatsApp → permanent token + phone number id.
2. Expose the gateway (ZeroClaw `[tunnel]` or your own proxy).
3. Meta Callback URL: `https://<public-host>/whatsapp/default` (match your
   alias), same `verify_token`, subscribe to `messages`.

This breaks the lean “no published ports” default. Prefer Web mode for friends
on a home server.

---

## Config checklist

WhatsApp is **opt-in** (example config is Telegram-only):

1. `[channels.whatsapp.default]` — **either** `session_path` (Web) **or**
   `phone_number_id` (Cloud), not both
2. Web: `mode = "personal"` + allowlist policies
3. `[peer_groups.whatsapp_default]` — `agents = ["main"]` + E.164 peers
4. `[agents.main].channels` — include `"whatsapp.default"`
5. `make remote-deploy` → link device → `channel doctor` → DM → group

`.env` does **not** sync WhatsApp peers yet (Telegram only). Edit
`config/config.toml` or `zeroclaw config set` over SSH.

---

## Security

- Keep `external_peers` tight — open WhatsApp is easy to spam.
- Prefer `mention_only = true` in groups.
- Prefer a **dedicated number** so personal chats stay off LOCAL_AGENT’s session.
- Session DB is credentials — under gitignored `config/` / `data/`.
- Cloud API tokens: env / secrets only, never commit.

---

## Troubleshooting

| Symptom | Likely fix |
|---|---|
| No QR / “web mode unavailable” | Image lacks `whatsapp-web`; confirm `session_path` and `ZEROCLAW_BASE` features |
| Friend ignored | Add E.164 to `peer_groups.whatsapp_default.external_peers`; `agents = ["main"]` |
| Policies ignored | Need `mode = "personal"` (default `business` skips DM/group policy split) |
| Group ignored | `group_policy`, `mention_only`, or `allowed_groups` (exact JID / user part) |
| Relink every restart | `session_path` not on a persistent volume |
| Both modes weird | Don’t set `session_path` and `phone_number_id` on the same alias |
| Cloud webhook 403 / silence | Tunnel URL `/whatsapp/<alias>`, `verify_token`, `messages` subscription |

Telegram: [docs/telegram.md](telegram.md). SMS alternatives: [docs/sms.md](sms.md).
Deploy: [docs/deploy.md](deploy.md).
