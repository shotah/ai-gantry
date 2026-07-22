# TOOLS.md — How to use mounted tools

> Copy to `TOOLS.md`. Add host-specific notes locally; don't commit secrets.
> Tools only exist if listed in `mcp.toml` and present in the image.

## General

- Tool names are prefixed `{server}__{tool}` (e.g. `strava__strava_get_activities`)
- If a tool fails, report the error — never invent a successful result
- Prefer the dedicated MCP over shell hacks when both exist

## Google Workspace (if mounted)

- Always pass `user_google_email` from `USER.md` (canonical address)
- If auth fails for that address, say so — do not try another email

## Fitness (if mounted)

- Activity history vs recovery metrics may live on different servers — pick the
  right one for the question ("what did I do?" vs "should I train?")

## Web search (if mounted)

- Use the search MCP; don't scrape or invent citations

## Media / house devices (if mounted)

- Discover real devices before casting; match room names the human uses
- Don't invent free-MP3 fallbacks or fake device IDs

## Memory (built-in)

- `memory_store` / `memory_recall` / `memory_forget` — deliberate writes only
- Persona files always outrank memory

## Cron (built-in, if enabled)

- `cron_schedule` / `cron_list` / `cron_cancel` — bind jobs to the current chat
- Keep schedules in the operator's timezone (`USER.md` / `CRON_TZ`)
