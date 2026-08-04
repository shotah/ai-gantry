# Cron / scheduled turns

Proactive jobs live in SQLite and fire inside the gantry: run the normal agent
loop (MCP tools allowed), then **push** the reply on Telegram (or print on
stdio). Pure-MCP cron cannot deliver outbound chat by itself.

## Config

| Env | Default | Meaning |
| --- | --- | --- |
| `CRON_ENABLED` | `true` | Master switch |
| `CRON_TZ` | `UTC` | IANA timezone for clock times (`America/Los_Angeles`) |
| `CRON_MAX_JOBS` | `50` | Cap on enabled jobs |
| `CRON_TICK_SECONDS` | `15` | Due-job poll interval |
| `SPARK_QTY` | _(empty)_ | **Opt-in** spark-of-life. Empty = off. `5` or `4-6` (random count/day) |
| `SPARK_START_HOUR` | `6` | Local window start (inclusive), used when spark is on |
| `SPARK_END_HOUR` | `21` | Local window end (exclusive), e.g. 21 → last ping before 9pm |
| `SPARK_PROMPT` | _(built-in check-in pool)_ | One prompt, or one per line (`\n`) — random pick per ping |
| `SPARK_SKIP_RECENT_MINUTES` | `15` | Skip/defer if the human messaged within this many minutes |

## Builtin tools

| Tool | Purpose |
| --- | --- |
| `cron_schedule` | Create a job bound to the current chat/session |
| `cron_list` | List active jobs |
| `cron_cancel` | Disable by id |

### `when` / `repeat`

| when | repeat | Result |
| --- | --- | --- |
| `in 30m` | `once` (default) | One-shot relative |
| `17:00` | `once` | Next 5pm in `CRON_TZ` |
| `17:00` | `daily` | Every day at 5pm |
| `every:1h` | — | Interval from now |
| RFC3339 | `once` | Absolute UTC/offset time |
| `4-6@06-21` | `spark` | Random 4–6 pings/day between 6am and 9pm |

Example prompts the model can schedule:

```text
Remind me at 5pm to submit my timecard.
At 5pm daily: summarize calendar + work email for the past 8 hours.
```

## Spark of life (opt-in)

Random presence pings — short authentic check-ins, not ops digests.
**Off unless `SPARK_QTY` is set.**

On Telegram, boot auto-binds a spark **planner** per allowlisted DM (`chat_id` =
user id) and seeds that day's pings. Other channels: `cron_schedule`
(`repeat=spark`, `when=4-6@06-21`).

How it works:

1. A daily `spark` planner wakes at window start (and on boot for the remaining day).
2. It rolls qty in `[min, max]` and inserts that many one-shot `spark_ping` jobs,
   spaced across the remaining window so the day stays balanced and the minimum is hit.
3. Before each seed (planner wake or boot catch-up), pending `spark_ping` rows for that
   session are cancelled — prior-day leftovers and restarts do **not** compound.
   Once today is planned (planner `next_run` is tomorrow), reboot does not roll a second set.
4. Each ping picks one line from `SPARK_PROMPT` (if multi-line) and runs the agent;
   if the human messaged within `SPARK_SKIP_RECENT_MINUTES`, that ping is deferred
   once, then dropped if still chatting.

```env
SPARK_QTY=4-6
SPARK_START_HOUR=6
SPARK_END_HOUR=21
# Optional: one prompt, or one variant per line (random pick per ping):
# SPARK_PROMPT=Generate a short Spark of Life check-in. Keep it under 3 sentences. No tools. …
# SPARK_SKIP_RECENT_MINUTES=15
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

One-shot jobs disable after a successful (or failed) fire. Daily/every advance
`next_run_at`. Push failures are recorded in `last_error`.
