# slash_commands — TODO (chatops as the interface)

The chat is the console: allowlisted, authenticated, already on the phone,
and already the place where slowness is *felt*. These commands expose what
the kernel already knows — its own loop accounting, its own SQLite, its own
process. **Never scrape the host** (GPU, `ollama ps`, journald): that stays
in [docs/observability.md](docs/observability.md). Commands stay separate
while there are few of them; merge later only if the list gets overloaded.

Shipped: `/new` `/cancel` `/status` `/tools` `/perf` `/memstats` `/toolstats`
`/auth` `/help`.

Conventions (all commands):

- Never touch the model — pure kernel-side reads, answer in <10ms.
- Take the session lock like `/status` does (`/cancel` stays the exception).
- Plain text, grep-able `key=value` / aligned-column style matching `/status`.
- Stay under one Telegram message (4096 chars) — cap lists, no pagination.
- Ephemeral counters die with the process; the journal is the durable record.
  Slash output is the last-mile view, not a second store.

---

## P0 — `/perf` (why was that turn slow?)

**What:** the last N turns' timing split, readable from the phone right after
a turn felt slow — today this answer requires SSH + `journalctl`.

**Returns** (newest first, one line per turn, ~12 turns max):

```text
perf — last 8 turns (uptime 2h13m)
#8 09:02:41  total=14.9s model=13.1s tool=1.8s iters=2 first_token=9.8s volatile≈1.4k
#7 08:57:12  total=6.2s  model=6.2s  tool=0s   iters=1 first_token=2.1s volatile≈0.3k
#6 08:41:03  total=76.4s model=74.1s tool=2.3s iters=3 first_token=68.2s volatile≈0.9k  ← cold
```

Read it as: `first_token` ≈ prefill (cache miss when it balloons),
`model` vs `tool` = which half to attack, `volatile` = the token estimate
that predicts prefill ([deploy-native.md § Latency](docs/deploy-native.md#latency-measure-before-tuning)).

**How:**

- [x] `perfRing` in `internal/agent`: fixed-size ring buffer (default 12,
      const not env — no new knob), mutex-guarded, global across sessions
- [x] Record struct: `when`, `totalMS`, `modelMS`, `toolMS`, `iters`,
      `firstTokenMS` (from iteration 1's model call; 0 = non-streaming),
      `volatileEst` (iteration 1), `source` (user / cron / reaction)
- [x] Append in the existing `turn perf` defer in `runLoop` — same numbers,
      one extra consumer; slash commands themselves never append
- [x] Mark the first turn after boot `← cold` (the `warmed` atomic already
      distinguishes it)
- [x] `case "/perf":` in `Handle` + render; empty ring → `no turns yet`
- [x] Test alongside the `/status` test in `agent_test.go`: run two fake
      turns, assert order (newest first), cap, and cold marker

---

## P1 — `/memstats` (what has he remembered?)

**What:** the health of the memory subsystem without `sqlite3` access —
row counts, decay, and whether consolidation is keeping up.

**Returns:**

```text
memory: 214 rows  fact=88 preference=41 person=23 insight=19 episode=43
state: active=196 expired=9 superseded=9
consolidation: backlog=12 episodes  quarantined=1  last_run=09:00:14 ok
db: 1.4 MB (WAL)
```

**How:**

- [x] Counts: `SELECT kind, COUNT(*) … GROUP BY kind` + active/expired/
      superseded — one query pass on the already-open handle
- [x] Backlog = unconsolidated `episode` rows; quarantined = rows past the
      3-attempt limit (see readme §6.3)
- [x] `last_run` needs the consolidator to keep an in-memory
      `lastRun time.Time` + `lastErr error` — expose via a small getter;
      `MEMORY_BACKEND=mcp:…` or consolidation off → print `consolidation: off`
- [x] db size: `PRAGMA page_count` × `PRAGMA page_size` (skip stat of -wal;
      estimate is fine and labeled by the WAL suffix)
- [x] `MEMORY_ENABLED=false` → `memory: disabled`
- [x] `case "/memstats":` + test with a seeded store

---

## P1 — `/toolstats` (which MCP is the chronic offender?)

**What:** cumulative per-tool counters since boot. `/perf` shows one slow
turn; this shows the pattern. Kept separate from `/tools` (that's the
catalog; this is the ledger).

**Returns** (sorted by total time spent, top ~15):

```text
tool stats — 23 calls since boot (2h13m)
google__gmail_search     6 calls  ✓5 ✗1  avg 2.8s  max 9.4s
garmin__sleep_get        9 calls  ✓9 ✗0  avg 1.2s  max 3.1s
repairs: prefix_alias=3  constrained_retry=1  unknown_tool=2
```

**How:**

- [x] Counter map in the MCP host (`internal/mcp`): per resolved tool name —
      `calls`, `errors`, `totalDur`, `maxDur`; mutex-guarded, reset on boot
- [x] Increment where `tool done` / `tool call failed` are logged (one site,
      same source of truth as the journal)
- [x] Repair counters: prefix alias hits, constrained retries issued,
      unresolvable names — these quantify the name-repair story the readme
      sells, and today they're only visible by grepping logs
- [x] `case "/toolstats":` + render; no calls yet → `no tool calls yet`
- [x] Test: fake host, two calls one error, assert counts and sort order

---

## P2 — `/status` additions (keep it one line)

**What:** `/status` stays the one-glance line; add only what fits that shape.

**Returns** (append to the existing line):

```text
uptime=2h13m model=qwen3.6:35b-a3b history_messages=41 history_est_tokens=9k
tools=62 schema_est_tokens=11k turns=23 rss_mb=48
```

**How:**

- [x] `turns` counter — trivial once the `/perf` ring exists (total appended,
      not ring length)
- [x] `rss_mb` — read `/proc/self/status` `VmRSS` (Linux-only; print nothing
      on other GOOS rather than `n/a` noise). **Honesty note:** this is the
      kernel binary only — MCP children are separate processes; the full
      stack number stays `systemctl status` / `docker stats`
      ([observability.md](docs/observability.md)); consider `rss_self_mb` if
      ambiguity bites

---

## P2 — `/auth <server>` (remote OAuth without a port)

**What:** today OAuth requires a workstation next to a browser
(`make google-auth`) and an scp of tokens — the single biggest gap for
friends running their own instance on a headless box. Two flows fix it with
**zero inbound ports**, both driven from chat:

1. **Device flow (RFC 8628)** — already shipped for YouTube (TV/Limited
   Input client): bot posts `verification_url` + `user_code`, user taps it
   on their phone, MCP polls the token endpoint. No callback exists at all.
   Google restricts device-flow scopes (YouTube/Drive-file yes; **Gmail /
   Calendar no**), so it can't cover Workspace.
2. **Auth-code paste with PKCE** — for everything device flow can't reach
   (Workspace, Strava): `redirect_uri` points at a **static** catch page
   (GitHub Pages — https, no server logic) that just displays the `?code=`
   with a copy button. User pastes it back into the chat; the MCP exchanges
   it. PKCE makes a code transiting Telegram useless without the verifier
   the MCP holds; codes are single-use with ~10 min TTL anyway.

**Returns / flow:**

```text
you:  /auth strava
bot:  open https://www.strava.com/oauth/authorize?...&redirect_uri=https://shotah.github.io/ai-gantry/oauth-catch/&code_challenge=...
      then paste the code here: /auth strava <code>
you:  /auth strava 9f3c2e…
bot:  strava: authorized ✓ (tokens → data/.strava/)
```

**How:**

- [x] Extend the shared MCP auth contract ([docs/auth.md](docs/auth.md)):
      alongside interactive `auth`, add `auth url` (print authorize URL +
      hold PKCE verifier/state) and `auth exchange <code>` — per-MCP work
      (google-mcp, go-strava-mcp, google-health-mcp; youtube device start/wait)
- [x] Static catch page HTML in-repo ([docs/oauth-catch/](docs/oauth-catch/index.html));
      publish to GitHub Pages (`https://shotah.github.io/ai-gantry/oauth-catch/`) and
      register as redirect URI on the Google/Strava/Health apps
- [x] `case "/auth":` in `Handle`: no code → run `auth url`, post it;
      with code → run `auth exchange`, confirm; unknown server → list
      manifest servers with `auth_args`
- [x] State/verifier lives in a pending file next to MCP tokens (10 min TTL;
      `/auth <server>` restarts) — not in a long-lived process
- [x] **Never** prompt for passwords in chat — Garmin's session login stays
      `make garmin-auth` + sync; `/auth garmin` should say exactly that
- [x] Rejected: outbound tunnel (cloudflared / tailscale funnel) for a real
      callback — works, but adds a running dependency and violates the
      spirit; the static page keeps "no server, no port" literal

---

## P3 — polish

- [x] Register the command list with BotFather (`/setcommands`) so Telegram
      autocompletes them — doc note in
      [local-agent/docs/telegram.md](local-agent/docs/telegram.md), not code
- [x] Update the slash-command lists: readme §7 Ops surface, stdio banner in
      `internal/channel/stdio` (`/new /status /tools /quit`), telegram.md
- [x] Add a "from the chat" row to the observability cheat sheet: slow turn →
      `/perf`, memory health → `/memstats`, tool offender → `/toolstats`
- [x] `/help` listing the commands one-per-line — cheap, and stdio/Discord
      users don't get BotFather autocomplete

---

## Non-goals (the line that keeps this small)

- No `/gpu`, `/ollama`, `/journal` — the kernel talks to an OpenAI-compat
  endpoint precisely so it doesn't know what serves it; host-side stays in
  [docs/observability.md](docs/observability.md)
- No persistence for counters/rings — restart wipes them by design
- No new env knobs — ring size and list caps are constants until proven wrong
- No formatting beyond plain text — grep-able beats pretty
