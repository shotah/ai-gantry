# Cron / scheduled turns

Proactive jobs live in SQLite and fire inside the gantry: run the normal agent
loop (MCP tools allowed), then **push** the reply on Telegram (or print on
stdio). Pure-MCP cron cannot deliver outbound chat by itself.

Live-data jobs (calendar, mail, fitness, search, sheets) get a tool-first
wrapper plus a last-token system note so the model calls tools before drafting
the digest. If it still writes the report with zero tool calls, the agent loop
nudges once; a second no-tool draft is refused instead of shipping invented
metrics. Prior `[cron]` turns are omitted from that job's prompt so yesterday's
digest cannot few-shot the next one. Plain reminders ("submit my timecard")
are unchanged.

Cron has no Telegram streaming / tool-trace bubble — only the final `Push`.
Live-data replies append `— tools: name, …` or `— tools: (none)` so a skipped
pull is visible in chat. Server logs still show `tool call` / `model call`.

The model can skip the push by replying with `[silent]` (first line). The job
still runs and the turn is stored; nothing is sent to chat. Use that for
all-clear / work-only jobs (dead-man, health checks) and for spark when a
check-in would be noise.

## Config

| Env | Default | Meaning |
| --- | --- | --- |
| `CRON_ENABLED` | `true` | Master switch |
| `CRON_TZ` | `America/Los_Angeles` | IANA timezone for clock times (Pacific — SJ / SF / SEA / LA) |
| `CRON_MAX_JOBS` | `50` | Cap on enabled jobs |
| `CRON_TICK_SECONDS` | `15` | Due-job poll interval |
| `SPARK_QTY` | _(empty)_ | **Opt-in** spark-of-life. Empty = off. `5` or `4-6` (random count/day) |
| `SPARK_START_HOUR` | `6` | Local window start (inclusive), used when spark is on |
| `SPARK_END_HOUR` | `21` | Local window end (exclusive), e.g. 21 → last ping before 9pm |
| `SPARK_PROMPT` | _(built-in check-in pool)_ | One prompt, or one per line (`\n`) — random pick per ping |
| `SPARK_SKIP_RECENT_MINUTES` | `30` | Skip/defer if the human messaged within this many minutes |
| `EXAMPLES_QTY` | `1-2` | **On by default** capability-example pings. Empty or `0` = no proactive pings. `/examples` on-demand still works |
| `EXAMPLES_START_HOUR` | `6` | Local window start for examples pings |
| `EXAMPLES_END_HOUR` | `21` | Local window end (exclusive) |
| `EXAMPLES_SKIP_RECENT_MINUTES` | `60` | Skip/defer if the human messaged within this many minutes |

## Builtin tools

| Tool | Purpose |
| --- | --- |
| `cron_schedule` | Create a job bound to the current chat/session |
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
| `4-6@06-21` | `spark` | Random 4–6 presence pings/day between 6am and 9pm |
| `1-2@06-21` | _(boot)_ | Examples planner uses the same qty@HH-HH shape (`examples` / `examples_ping` kinds) |

Example prompts the model can schedule:

```text
Remind me at 5pm to submit my timecard.
At 5pm daily: summarize calendar + work email for the past 8 hours.
At midnight daily: check last 48h of chat + Garmin. If all-clear, reply [silent].
```

## Spark of life (opt-in)

Random presence pings — short authentic check-ins, not ops digests.
**Off unless `SPARK_QTY` is set.**

On Telegram, boot auto-binds a spark **planner** per allowlisted DM (`chat_id` =
user id) and seeds that day's pings. Other channels: `cron_schedule`
(`repeat=spark`, `when=4-6@06-21`).

How it works:

1. A daily `spark` planner is seeded on boot for the remaining day, then wakes again at
   **tomorrow's** window start (not a second roll for today once `next_run` is tomorrow).
2. It rolls qty in `[min, max]` and inserts that many one-shot `spark_ping` jobs,
   spaced across the remaining window so the day stays balanced and the minimum is hit.
3. Before each seed (planner wake or boot catch-up), pending `spark_ping` rows for that
   session are cancelled — prior-day leftovers and restarts do **not** compound.
   Once today is planned (planner `next_run` is tomorrow), reboot does not roll a second set.
4. Each ping picks one line from `SPARK_PROMPT` (if multi-line) and runs the agent;
   if the human messaged within `SPARK_SKIP_RECENT_MINUTES`, that ping is deferred
   once, then dropped if still chatting. The agent can also reply `[silent]` to
   skip the push when a check-in would feel like noise.
5. Cancelling the spark planner (`cron_cancel`) also disables pending pings for that session.

```env
SPARK_QTY=4-6
SPARK_START_HOUR=6
SPARK_END_HOUR=21
# Optional: one prompt, or one variant per line (random pick per ping):
# SPARK_PROMPT=Generate a short Spark of Life check-in. Keep it under 3 sentences. No tools. …
# SPARK_SKIP_RECENT_MINUTES=30
```

## Capability examples / training wheels (on by default)

Inventory-aware multi-step ideas (propose only — no tools on the ping).
**On unless `EXAMPLES_QTY` is empty or `0`.** Default is `1-2` pings/day.

Chat controls:

| Command | Effect |
| --- | --- |
| `/examples` | One suggestion now (filtered to connected MCP servers) |
| `/examples on` / `true` | Re-enable proactive pings for this chat |
| `/examples off` / `false` | Opt out (persists across restarts) |

On Telegram, boot auto-binds an examples **planner** per allowlisted DM (same
session shape as spark), skipping sessions that opted out. Pings pick a curated
seed whose required server prefixes are all present in the live `/tools` catalog,
then ask the model to localize it. Turn off anytime with `/examples off`.

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
`next_run_at`. Push failures are recorded in `last_error`.
