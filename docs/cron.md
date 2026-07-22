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

Example prompts the model can schedule:

```text
Remind me at 5pm to submit my timecard.
At 5pm daily: summarize calendar + work email for the past 8 hours.
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
