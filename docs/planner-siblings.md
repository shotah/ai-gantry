# Daily planner: pendant and gantree

The crane replaced Spark of life with one daily planning session. This
file is the checklist for the other checkouts. The crane side is
[cron.md](cron.md). The yard write-path is
[gantree-contract.md](gantree-contract.md).

Neither sibling has a new socket, frame kind, or `gantry status` field.
Work is copy, menus, and any screen that still names spark.

---

## Pendant (`gantry-pendant`)

The mailbox already receives the new menu. On connect the crane sends
`kind: "cmds"` from `slash.Catalog()` (`internal/channel/pendant/inbound.go`).
That list now has `planner` (`args: true`, hint `daily planning session
(on|off|HH:MM)`) and does not have `spark` or `engagement`.

Chat pushes are still `{ kind: "push", text }`. A planning session that
speaks starts with `[cron] Daily planner`. A `[silent]` session sends
nothing.

Update the PWA only where a string is hardcoded:

| Surface | Change |
| --- | --- |
| Command picker, icons, or tests that name `/spark` or `/engagement` | `/planner`, with an argument (`on`, `off`, or a clock like `09:30`) |
| Any match on `[cron] Spark of life` | `[cron] Daily planner` |
| Room, theme, face, allowlist, reactions | Leave them |

A picker that renders the `cmds` frame as-is needs no release for the
new command. Cab uses the same mailbox and the same rule.

---

## Gantree (`gantree`)

The crane boots without a new required env var. `gantry status` JSON is
unchanged. Planner turns still log `msg=turn perf` with `source=cron`.

### Persona template

`examples/persona/PERSONA.example.md` in this repo is the seed. The yard
template that **Replace from template** ships has to match it, including
the `[cron] Daily planner` example and the `cron / daily planner` layer.
Drop Spark of life, `/spark`, and `/engagement`. Seed tests in the yard
should pin the planner wording the way they already pin `pref/hours` and
`pref/calendar`.

### Env form

Optional, same file as the other cron knobs (`.env.example`):

| Key | Default | Meaning |
| --- | --- | --- |
| `DAILY_PLANNER_AT` | `07:10` | Local clock (`CRON_TZ`). `HH:MM` or `H:MM`. |

There is no spark quantity env to remove. `/planner 09:30` on the crane
overrides this for that agent and is stored in SQLite, not in `.env`.

### SQLite the board may already read

`/data/gantry.db`:

| What | Now |
| --- | --- |
| `cron_job.kind` | `daily_planner` for the one session. `expr` is `HH:MM` (for example `07:10`). |
| `cron_job.kind` | `examples` / `examples_ping` are unchanged. |
| `session_pref.planner_at` | `''` inherit `DAILY_PLANNER_AT`, `0` off, or `HH:MM` override. Added with `ALTER TABLE`; new databases create the column directly. |

This binary does not read `spark_qty` and does not rewrite old `spark` /
`spark_ping` rows. A board that filters on those kinds will not show the
daily session. Label `daily_planner` as the planning session. Leave
`spark_qty` alone.

### Docs and UI copy

Anywhere the yard still says Spark of life, `/spark`, `/engagement`,
`repeat=spark`, or “3–5 wakes a day”, switch it to one clock,
`DAILY_PLANNER_AT`, `/planner`, and `cron_schedule` `repeat=planner`.
