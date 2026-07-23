# Persona & system prompt (LOCAL_AGENT)

gantry builds LOCAL_AGENT's system prompt each session from markdown in:

```text
persona/
```

That directory is bind-mounted read-only at `/persona` in the container.
Files concatenate in a fixed order (`SOUL.md`, `IDENTITY.md`, `USER.md`,
`AGENTS.md`, `TOOLS.md`, `HEARTBEAT.md`, `BOOTSTRAP.md`, `MEMORY.md`);
missing files are skipped, and any other `*.md` follows alphabetically.

## Templates vs personal files

| Committed (safe) | Local / server (gitignored) |
| --- | --- |
| `SOUL.example.md` | `SOUL.md` |
| `USER.example.md` | `USER.md` |
| `IDENTITY.example.md` | `IDENTITY.md` |
| `AGENTS.example.md` | `AGENTS.md` |
| `TOOLS.example.md` | `TOOLS.md` |
| `MEMORY.example.md` | `MEMORY.md` |
| `HEARTBEAT.example.md` | `HEARTBEAT.md` |

```bash
make persona          # create missing *.md from *.example.md
make persona-force    # overwrite *.md from examples (wipes local edits)
```

`make init` and `make remote-sync` run `persona` so files exist before deploy.
Fill in **`USER.md`** (name, canonical Google email, city) before expecting good Google tool calls.

## What each file is for

| File | Purpose |
| --- | --- |
| `SOUL.md` | Personality, coach mode, anti-hallucination rules |
| `USER.md` | Who you are — **canonical email lives here** |
| `IDENTITY.md` | Name "LOCAL_AGENT", vibe |
| `AGENTS.md` | Operating rules / workflows |
| `TOOLS.md` | How to use Google / Strava / Garmin / Cast / YT Music / memory |
| `MEMORY.md` | Curated long-term notes (injected every session) |
| `HEARTBEAT.md` | Optional periodic checks (empty = skip) |

These are **not** the same as gantry's SQLite memory (`data/gantry.db`).
Persona files = doctrine you control, loaded every session. SQLite memory =
recall LOCAL_AGENT writes deliberately (`memory_store`) and can get wrong.
**Persona precedence is law**: anything in `USER.md` outranks recalled memory.

## Edit & deploy

```bash
# edit persona/*.md  (gitignored)
make remote-sync          # ensures persona files exist, then scp
make remote-restart       # or remote-up if down
```

After a bad session: Telegram `/new`, and scrub bad memory rows if needed
(`make shell` → `sqlite3 gantry.db` or ask LOCAL_AGENT to `memory_forget`).

## Related

- Models / provider swap: [models.md](models.md)
- Telegram `/new` + memory notes: [telegram.md](telegram.md)
