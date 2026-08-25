# Inspecting gantry memory with `sqlite3`

> Design context: [design.md](design.md) · [docs index](README.md)

Builtin memory is how the harness keeps **long-horizon** facts: the same
SQLite file as sessions, greppable and deletable.

```text
$DATA_DIR/gantry.db
```

Default in Docker: `/data/gantry.db`.

## Open the DB

```bash
sqlite3 /data/gantry.db
```

Useful pragmas once inside:

```sql
PRAGMA journal_mode;   -- expect wal
.tables
.schema memory
```

## List active memories

```sql
SELECT id, kind, subject, content, source, created_at, expires_at, superseded_by, consolidated
FROM memory
WHERE superseded_by IS NULL
  AND NOT (kind = 'episode' AND consolidated != 0)
  AND (expires_at IS NULL OR expires_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ORDER BY updated_at DESC
LIMIT 50;
```

## Full-text search (FTS5)

```sql
SELECT m.id, m.kind, m.subject, m.content
FROM memory_fts
JOIN memory m ON m.id = memory_fts.rowid
WHERE memory_fts MATCH '"chris" OR "climbing"'
  AND m.superseded_by IS NULL;
```

## Fix a wrong row by hand

```sql
-- delete one id (same as memory_forget)
DELETE FROM memory WHERE id = 42;

-- or soft-retire via supersede (consolidator style)
UPDATE memory
SET superseded_by = 99, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = 42;
```

FTS triggers keep `memory_fts` in sync on insert/update/delete.

## Episodes vs durable rows

| kind | typical TTL | notes |
|------|-------------|--------|
| `episode` | 30 days | raw notes; consolidator reads these |
| `fact` / `preference` / `person` / `insight` | none | durable; hydrated into the prompt. Months-scale plans: `insight` with subject `aim/<area>` ([persona.md](persona.md#where-the-horizon-lives)). |

`consolidated = 1` means the consolidator already processed that episode (hidden from
hydrate/recall). `consolidated = 2` is quarantined after repeated parse failures.
`MEMORY_CONSOLIDATE_MINUTES` only runs for `MEMORY_BACKEND=builtin`.

Durable `memory_store` (fact / preference / person / insight) with the **same
kind+subject** inserts a new row and supersedes the old live one. Hydrate and
recall skip superseded rows. Use that to correct "doesn't like sushi" → "likes
sushi now". Episodes with the same subject do **not** collapse (the consolidator
needs the pile). `memory_forget` still hard-deletes.

`pref/hours` is the structured sleep/work/quiet row (`sleep: 23:00-07:00` …).
The agent stamps `[hours]` on every turn from it. Spark/examples skip during
the sleep window; explicit user crons still fire.

## Session vs memory

`/new` clears `session_message` for that chat. It does **not** touch `memory`.

```sql
SELECT COUNT(*) FROM session_message WHERE session_id = 'telegram:123';
SELECT COUNT(*) FROM memory;
```

## Config knobs

| env | meaning |
|-----|---------|
| `MEMORY_ENABLED` | `true`/`false` |
| `MEMORY_BACKEND` | `builtin` or `mcp:<server>` |
| `MEMORY_CONSOLIDATE_MINUTES` | timer interval; `0` disables (builtin only) |
