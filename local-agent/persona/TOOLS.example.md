# TOOLS.md — How to use tools here

> Copy to `TOOLS.md` via `make persona`. Add host-specific notes locally; don’t commit secrets.

## Server routing (read first)

| Ask about… | Server | Never |
|---|---|---|
| Calendar / mail / tasks / docs / sheets | `google__…` | `strava__`, `garmin__` |
| Workouts / load | `strava__` / `garmin__` | `google__` |
| Sleep / recovery | `garmin__` | `strava__` for sleep |

Wrong-server error → switch prefix; don’t retry Strava for calendar.

## Timezone (all tools)

- Human timezone is in `USER.md` / `[current time]` (from `CRON_TZ`) — usually Pacific here. Match that; not UTC.
- Date-only args: use the local calendar day from `[current time]` (`yesterday=` / `today=`).
- RFC3339 / timestamps: include that zone’s offset from `[current time]`. **Never default to `Z` / UTC** unless asked or the schema requires it.

## Google (MCP server `google`)

Tools: `google__{service}_{verb}_…` (e.g. `google__calendar_list_events`). Not fitness tools.

- Always pass `user_google_email` from `USER.md`
- Auth fails → `make google-auth` / `gantry auth google` — do not try another email
- Never invent mail/calendar/docs/sheets/tasks without a successful tool result
- **Calendar create:** `google__calendar_create_event` (not `calendar_update_event` — that needs `event_id`)
- **Calendar day:** `google__calendar_list_events` — omit `event_id`; `calendar_id="primary"`, both `time_min` + `time_max`
- **Calendar update:** `calendar_list_events` → id → `google__calendar_update_event`
- **Gmail:** `google__gmail_search_messages` → `google__gmail_get_message`; send only if asked
- **Tasks:** `google__tasks_list_tasks` / `google__tasks_create_task` (`task_list_id="@default"` when needed)
- **Docs / Sheets:** `google__docs_*` / `google__sheets_*` (everyday preset has no Drive tools)
- Do not narrate — call the exact `google__…` name from the tools list

## Math

- **Math MCP (`mcp-go-math`)** — `evaluate` for arithmetic; `convert` for units
- Use the math tool for **any arithmetic beyond trivial** — do not invent results in reasoning
- Prefer `evaluate` over mental math for percentages, multi-step formulas, and roots/powers

## Fitness

- **Strava MCP** — activities, load, weekly summaries (`strava__strava_get_activities`, by-id / zones / athlete stats)
- **Garmin MCP** — sleep, weight, Body Battery / HRV / readiness, **and** activity history (`garmin__list_activities`, `garmin__get_activity`)
- Prefer Garmin for recovery
- **“What did I do?” / yesterday’s ride / session:** call a list tool first — `garmin__list_activities` and/or `strava__strava_get_activities` (whichever is in the tools list). Bound the day in the human’s timezone from `[current time]` (not UTC). Then by-id detail if needed. Do **not** ask the human to paste stats when a list tool exists; do **not** invent “no daily activity tool” because you only looked for `get_activities`.
- **Sleep:** `garmin__get_sleep` only — not `get_body_battery`. Call it (don’t narrate “I’ll pull…”). For “last night” omit `date` or pass **today** (wake-up day). Example: today 2026-07-28 → `date=2026-07-28`, never yesterday

## Web search

- **Exact tool name:** `google-search__google_search` (not `web_search*`)
- New event + place → search if needed, then **`create_event`**. Existing + place → **`modify_event`**
- If two cities share a gym-ish name, prefer the city in the ask / `USER.md`

## YouTube Music

- **YT Music MCP (`youtube-go-mcp`, Go)** — search, library playlists, liked songs, history, radio, lyrics
- Prefer this over inventing royalty-free / stock music URLs
- Returns `videoId` → hand off to Cast `beam_youtube_video` (bare id, not a watch URL)
- Library tools need `make ytmusic-auth` (browser headers)

## House Cast (speakers / displays)

- **Cast MCP (`mcp-beam`, Go)** — discover Chromecast / Nest / DLNA on the LAN; beam URLs or local files; YouTube-by-id; pause / resume / seek / stop / volume / mute
- Prefer Cast tools over shell hacks for speakers/TVs
- **Music flow:** YT Music → pick `videoId` → `beam_youtube_video` + room device — never invent free-MP3 fallbacks
- **Never** pass YouTube/Music watch URLs to `beam_media` (Nest connects, silence)
- Match the human’s **room name** to a local room→device map (fill in below after `make persona`), then `list_local_hardware` and pick the best-matching device `id`
- **Discovery defaults** (always pass these — slower Nest hubs can lose the race vs Mini/TV):
  - `timeout_ms`: **10000**
  - `include_unreachable`: **true**
  - If a known room device is missing, call `list_local_hardware` again a few seconds later (background mDNS cache), then map by room
- Volume: `set_beaming_volume` (0–100) / `mute_beaming` on an active session

### Room → devices (edit for your house)

| Room | Devices | Default target |
|---|---|---|
| Bathroom | … | … |
| Kitchen | … | … |
| Living room | … | … |
| Bedroom | … | … |

## Memory tools

- `memory_recall` — helpful, but **not** authoritative for the human’s email/name
- `memory_store` — only confirmed facts; never store a new identity for the human
- `memory_forget` — delete contradictions with `USER.md` when you find them

## Shell

- Prefer MCP/domain tools over shell hacking
- No destructive commands without asking
