# Persona & system prompt (LOCAL_AGENT)

gantry builds LOCAL_AGENT's system prompt each session from markdown in:

```text
persona/
```

That directory is bind-mounted read-only at `/persona` in the container (Docker)
or pointed at by `PERSONA_DIR` (native). Files concatenate in a fixed order:

```text
SOUL.md → SELF.md → RULES.md → USER.md → TOOLS.md
```

Missing files are skipped. Any other `*.md` follows alphabetically (avoid extras
unless you mean them — leftovers still get loaded).

Voice and ops lean on the [OpenClaw](https://github.com/openclaw/openclaw) workspace
templates (`SOUL` / `AGENTS` / `USER`), folded into gantry’s four files so small
local models aren’t flooded. Hard tool-discipline (“no lazy tools”) lives in
`RULES.md` — OpenClaw SOUL alone is too soft for that.

## The four files

| File | Purpose |
| --- | --- |
| `SOUL.md` | Who the agent is — name/vibe, OpenClaw-style core truths, lab energy, communication |
| `SELF.md` | Who the agent has **become** — agent-written (`self_note` tool + distill on `/new`); capped ~4KB; prune freely |
| `RULES.md` | Operating rules — **no lazy tools**, identity lock, execute bias, safety, memory hygiene |
| `USER.md` | Who Chris is — **canonical email lives here**, directives, gyms, prefs |
| `TOOLS.md` | MCP recipes only — exact tool names, Google/fitness/cast/search how-tos |

`SELF.md` is the one agent-writable file (`SELF_NOTES_ENABLED`, default on).
Compose mounts `./persona` writable so notes can grow; only that file is
agent-written. Mounting `:ro` (or a read-only host dir) auto-disables
self-notes at boot — look for `self-notes disabled` in the logs. Set
`SELF_NOTES_ENABLED=false` if you want a read-only persona on purpose.

**Audit it.** If the agent drifts into tone or rituals you don’t want, prune
or wipe `SELF.md` — details in
[../../docs/troubleshooting.md](../../docs/troubleshooting.md#selfmd--personality-drift).

Keep tool recipes **out** of SOUL/RULES/USER. Point at `TOOLS.md` instead.

## Templates vs personal files

| Committed (safe) | Local / server (gitignored) |
| --- | --- |
| `SOUL.example.md` | `SOUL.md` |
| `RULES.example.md` | `RULES.md` |
| `USER.example.md` | `USER.md` |
| `TOOLS.example.md` | `TOOLS.md` |

```bash
make persona          # create missing *.md from *.example.md
make persona-force    # overwrite *.md from examples (wipes local edits)
```

`make init` and `make remote-sync` run `persona` so files exist before deploy.
Fill in **`USER.md`** (name, canonical Google email, city) before expecting good Google tool calls.

## Persona vs SQLite memory

These files are **not** the same as gantry's SQLite memory (`data/gantry.db`).
Persona = doctrine you control, loaded every session. SQLite memory = facts the
agent writes via `memory_store` (and can get wrong).
**Persona precedence is law**: anything in `USER.md` outranks recalled memory.

There is no curated `MEMORY.md` anymore — put durable human facts in `USER.md`.

## Obsolete OpenClaw / ZeroClaw-era names

These are **not** used (removed on `make remote-sync` / native `rsync --delete`):

`IDENTITY.md`, `AGENTS.md`, `MEMORY.md`, `HEARTBEAT.md`, `BOOTSTRAP.md`

Map: OpenClaw `SOUL`+`IDENTITY` → `SOUL.md`; `AGENTS` → `RULES.md`; durable
facts that were in `MEMORY.md` → `USER.md`. (`HEARTBEAT.md` was never the SQLite
healthcheck — that lives in `gantry.db`.)

## Edit & deploy

```bash
# edit persona/*.md  (gitignored)
make remote-sync          # ensures persona files exist, scp, cleans obsolete names
make remote-restart       # or remote-up if down
# native: make remote-native-deploy-dev  (install.sh rsync --delete on persona/)
```

After a bad session: Telegram `/new`, and scrub bad memory rows if needed
(`make shell` → `sqlite3 gantry.db` or ask the agent to `memory_forget`).

## Related

- Models / provider swap: [models.md](models.md)
- Telegram `/new` + memory notes: [telegram.md](telegram.md)
