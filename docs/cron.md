# Cron, spark, and watches

Proactive jobs are **long-horizon harness work**: they live in SQLite and
fire inside gantry — run the normal agent loop (MCP tools allowed), then
**push** on the current CHANNEL (Telegram, pendant, Discord, Slack, or
stdio). Jobs do not store a destination. Switching `CHANNEL=telegram` to
`CHANNEL=pendant` keeps the 7am morning summary: same row, same `next_run`,
Push now uses the pendant allowlist. Pure-MCP cron cannot deliver outbound
chat by itself. A reminder next Tuesday is planning; a chatbot that only
answers now is not.

Live-data jobs (calendar, mail, fitness, search, sheets) get a tool-first
wrapper plus a last-token system note so the model calls tools before drafting
the digest. If it still writes the report with zero tool calls, the agent loop
nudges once; a second no-tool draft is refused instead of shipping invented
metrics. Prior `[cron]` turns are omitted from that job's prompt so yesterday's
digest cannot few-shot the next one. Plain reminders ("submit my timecard")
are unchanged.

A months-scale **aim** is not a cron by itself. North-star sentences live in
`SELF.md`; progress in memory (`aim/<area>`); cron is the wake. Spark looks
after the user — aims, live tools, filling useful personal knowledge, and a
joke when the data earns it. Empty zero-tool pings still stay `[silent]`.
Learned `pref/hours` sleep skips spark/examples; explicit "remind me at 9pm"
still fires. Pin follow-through with `memory_id` so the wake is not a hydrate
lottery. A goal with no wake is a dusty row — [persona.md](persona.md#where-the-horizon-lives).

Cron has no Telegram streaming / tool-trace bubble — only the final `Push`.
Live-data replies append `— tools: name, …` or `— tools: (none)` so a skipped
pull is visible in chat. Server logs still show `tool call` / `model call`.

The model can skip the push by replying with `[silent]` (first line). The job
still runs and the turn is stored; nothing is sent to chat. Use that for
all-clear / work-only jobs (dead-man, health checks) and for spark when the
work does not need a human-facing message.

## Config

| Env | Default | Meaning |
| --- | --- | --- |
| `CRON_ENABLED` | `true` | Master switch |
| `CRON_TZ` | `America/Los_Angeles` | IANA timezone for clock times (Pacific — SJ / SF / SEA / LA) |
| `CRON_MAX_JOBS` | `50` | Cap on enabled jobs |
| `CRON_TICK_SECONDS` | `15` | Due-job poll interval |
| `EXAMPLES_QTY` | `1-2` | **On by default** capability-example pings. Empty or `0` = no proactive pings. `/examples` on-demand still works |
| `EXAMPLES_START_HOUR` | `6` | Local window start for examples pings |
| `EXAMPLES_END_HOUR` | `21` | Local window end (exclusive) |
| `EXAMPLES_SKIP_RECENT_MINUTES` | `60` | Skip/defer if the human messaged within this many minutes |

## Builtin tools

| Tool | Purpose |
| --- | --- |
| `cron_schedule` | Create a job for this agent. Optional `memory_id` / `memory_subject` pins a memory row; the wake injects `[job memory]`. Push uses the CHANNEL allowlist. |
| `cron_list` | List active jobs |
| `cron_cancel` | Disable by id (spark planner also cancels pending `spark_ping` rows) |

### `when` / `repeat`

| when | repeat | Result |
| --- | --- | --- |
| `in 30m` | `once` (default) | One-shot relative |
| `17:00` | `once` | Next 5pm in `CRON_TZ` |
| `17:00` | `daily` | Every day at 5pm |
| `every:1h` | — | Interval from now |
| RFC3339 | `once` | Absolute UTC/offset time |
| `2-3@06-21` | `spark` | Random 2–3 horizon-planning wakes/day between 6am and 9pm |
| `1-2@06-21` | _(boot)_ | Examples planner uses the same qty@HH-HH shape (`examples` / `examples_ping` kinds) |

Example prompts the model can schedule:

```text
Remind me at 5pm to submit my timecard.
At 5pm daily: summarize calendar + work email for the past 8 hours.
At midnight daily: check last 48h of chat + Garmin. If all-clear, reply [silent].
```

## Spark of life (on by default)

Random **horizon wakes** — replan today against `SELF.md` north-stars and memory
`aim/`, call tools, and `cron_schedule` the next wake. Empty board: ask **one**
months-scale question (do not invent an aim), then `self_note` + `memory_store`
`aim/<area>` when they answer. **Off with `/engagement off`** (same as `/spark off`).

On boot, one spark **planner** is bound to the agent conversation (`gantry`).
`CHANNEL` is the mouth; jobs do not store a chat or user id. Push delivers to
every allowlisted destination on that mouth. Switching Telegram → pendant
keeps the same cron, spark pings, watches, memory, and history. Other
channels: same auto-bind (`/spark off` still opts out).

Chat controls (persist on the agent, like `/examples`):

| Command | Effect |
| --- | --- |
| `/engagement` / `/spark` | Same command. Status (default qty, this agent, window) |
| `/engagement on` | Inherit operator default |
| `/engagement off` | Opt out (dated user crons still fire) |
| `/engagement 2` / `/spark 4-6` | Override count/day (1–24) |

How it works:

1. A daily `spark` planner is seeded on boot for the remaining day, then wakes again at
   **tomorrow's** window start (not a second roll for today once `next_run` is tomorrow).
2. It rolls qty in `[min, max]` and inserts that many one-shot `spark_ping` jobs,
   spaced across the remaining window so the day stays balanced and the minimum is hit.
3. Before each seed (planner wake or boot catch-up), pending `spark_ping` rows for the
   agent are cancelled — prior-day leftovers and restarts do **not** compound.
   Once today is planned (planner `next_run` is tomorrow), reboot does not roll a second set.
4. Each wake picks one line from the built-in pool and runs the **full
   agent loop** (memory, cron, MCP tools). A zero-tool joke is nudged once; a second
   skip stays `[silent]` so it is not pushed. If the human messaged within
   skip-recent (30m), that wake is deferred once, then dropped if still chatting.
   Learned `pref/hours` sleep also defers spark/examples (work is not DND).
5. Work-only is the default: reply `[silent]` unless a hole needs the human (or the
   board is empty and it is time to ask once). Ask-first still applies (no email,
   spend, or public posts from a spark).
6. Cancelling the spark planner (`cron_cancel`) also disables pending pings.

## Capability examples / training wheels (on by default)

Inventory-aware multi-step ideas (propose only — no tools on the ping).
**On unless `EXAMPLES_QTY` is empty or `0`.** Default is `1-2` pings/day.

Chat controls:

| Command | Effect |
| --- | --- |
| `/examples` | One suggestion now (filtered to connected MCP servers) |
| `/examples on` / `true` | Re-enable proactive pings |
| `/examples off` / `false` | Opt out (persists across restarts) |

Boot auto-binds one examples **planner** for the agent (same conversation as
spark), skipping if opted out. Pings pick a curated seed whose required server
prefixes are all present in the live `/tools` catalog, then ask the model to
localize it. Turn off anytime with `/examples off`.

```env
EXAMPLES_QTY=1-2
EXAMPLES_START_HOUR=6
EXAMPLES_END_HOUR=21
# EXAMPLES_QTY=0   # disable proactive pings; /examples still works
# EXAMPLES_SKIP_RECENT_MINUTES=60
```

## Inspect with sqlite3

```bash
sqlite3 /data/gantry.db
```

```sql
SELECT id, kind, expr, timezone, next_run_at, enabled, running,
       substr(prompt, 1, 60), last_error
FROM cron_job
ORDER BY id DESC
LIMIT 20;
```

Disable by hand:

```sql
UPDATE cron_job SET enabled = 0, running = 0 WHERE id = 3;
```

## Overlap policy

Jobs run **serially** on the poller. A job sets `running=1` while the agent
turn executes; due rows that are still running are skipped until `Finish`.

On runner boot, any leftover `running=1` flags (crash/OOM mid-turn) are cleared so
jobs become due again. `Finish` / `Defer` only apply while `running=1` and never
re-enable a job that was cancelled mid-flight.

One-shot jobs disable after a successful (or failed) fire. Daily/every advance
`next_run_at`. Push failures are recorded in `last_error`. `cron_list` still
shows that row (`enabled=0`) — gone from the *enabled* set is not a delivery.

## Trace a wake (agent → channel → Cloudflare)

A cron fire is four hops. Job 319 disappearing from the enabled queue only
means `Finish` ran (one-shot disable, silent skip, empty reply, or push
error). It does not mean the phone got a frame.

```text
SQLite cron_job  →  runner.Handle (agent loop, session_id=gantry)
                 →  channel.Push  {kind:push, user_id from CHANNEL allowlist, id:cron-<job>-<ms>}
                 →  wss …/ws/<slug>?role=crane  (live socket, or a short dial)
                 →  Durable Object routes to tag sub:<user_id>  (or role:phone if empty)
                 →  phone WS + optional Web Push if that socket is gone
```

Grep gantry logs for the same `id` / `frame_id`:

```text
cron job firing     id=319 session_id=gantry
cron silent skip    outcome=silent          # Handle ran; nothing written to the mailbox
cron job pushed     outcome=push frame_id=cron-319-…
cron push failed    outcome=error           # last_error on the row
pendant push        slug=… user_id=… via=live|dial frame_id=cron-319-…
```

`via=dial` means the long-lived crane socket was down (or a live write
failed) and Push opened a one-shot mailbox connection. Jobs do not store a
Telegram chat id or Google `sub` — the mouth allowlist is the destination.
Boot collapse rewrites leftover `telegram:…` / `pendant:…` rows onto `gantry`.
Those jobs still fire — they are not skipped because they were scheduled on
another mouth. The Worker does not log frame bodies; `pendant push` is the
last crane-side hop you can grep. On the phone the bubble `id` is that
`frame_id`.

Disable by hand:

```sql
UPDATE cron_job SET enabled = 0, running = 0 WHERE id = 3;
```

---

## Event watches

A watch is a **cursor + poll**, not a chat loop. Quiet ticks call an MCP fetch
tool and **never** touch the Completer. New item ids wake the same agent loop
as cron, then **push** on the current CHANNEL — or skip if the reply is
`[silent]`. A `feeds__items_list` subscription added on Telegram still wakes
after `CHANNEL=pendant`. First poll seeds the cursor (no backlog dump). Do
not fake this with `cron_schedule` + “fetch the feed.”

```text
ticker → Host.CallRaw(tool, args) → compare ids → empty? stop
                                 → new? agent.Handle → Push (or [silent])
```

The poller uses `CallRaw`, not `Call`. `TOOL_RESULT_MAX_CHARS` is for the model;
cutting a feed JSON mid-string makes `ParseItems` fail and the watch never seeds.

Shares the cron ticker. Boot fails if watch is on and the channel cannot `Push`.

| Env | Default | Meaning |
| --- | --- | --- |
| `WATCH_ENABLED` | `true` | Master switch |
| `WATCH_MAX` | `50` | Cap on enabled watches |

| Tool | Purpose |
| --- | --- |
| `watch_add` | Subscribe: prefixed MCP `tool` + `args` + `interval` (default `15m`, min `1m`) + optional `label` |
| `watch_list` | List active watches |
| `watch_cancel` | Disable by id |

The poller does not know RSS vs Twitter. A watch row is `tool` + `args`.
Siblings return `{items:[{id,…}]}` JSON.

| Server | Binary | Tools | Watch args |
| --- | --- | --- | --- |
| `feeds` | [feeds-mcp](https://github.com/shotah/feeds-mcp) | `items_list`, `source_resolve` | `{ url }` |
| `twitter` | [twitter-mcp](https://github.com/shotah/twitter-mcp) | `posts_list` | `{ handle }` — prefer 30–60m (pay-per-use) |
| `boards` | [boards-mcp](https://github.com/shotah/boards-mcp) | `challenges_list` | `{}` or `{ "all": true }` — hour interval; do not watch `notices_list` |

Uncomment in [examples/mcp.toml.example](../examples/mcp.toml.example). Put
`X_BEARER_TOKEN` in `.env`, not in the manifest. Prior `[watch]` / `[cron]`
turns are omitted from the next scheduled prompt so they cannot few-shot the
next summary.
