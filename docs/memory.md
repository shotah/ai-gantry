# Inspecting gantry memory with `sqlite3`

> Design context: [design.md](design.md) · [docs index](README.md)

Builtin memory lives in the same SQLite file as sessions:

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
  AND (expires_at IS NULL OR expires_at > datetime('now'))
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
| `fact` / `preference` / `person` / `insight` | none | durable; hydrated into the prompt |

`consolidated = 1` means the consolidator already processed that episode.

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
