# TOOLS.md — How LOCAL_AGENT should use tools here

> Copy to `TOOLS.md` via `make persona`. Add host-specific notes locally; don’t commit secrets.

## Google Workspace (MCP)

- Always pass `user_google_email` from `USER.md` (canonical address)
- If auth fails for that address, say so and point at `make google-auth` — do not try another email
- Never invent message bodies, calendar events, or inbox contents without a successful tool result
- **Exact calendar tool name:** `google-workspace__get_events` (not `get_calendar_event`, not `list_events`)
- **Day / “what's on my calendar” queries — omit `event_id`.** Pass only:
  - `user_google_email` (from USER.md)
  - `calendar_id`: `"primary"`
  - **Both** `time_min` + `time_max` in RFC3339 using ISO dates from `[current time]`  
    e.g. today PT: `time_min="2026-07-28T00:00:00-07:00"`, `time_max="2026-07-29T00:00:00-07:00"`
  - Never omit `time_max` on purpose — unbounded lists drown you in future birthdays
  - Never put `"primary"`, a date, or a time range into `event_id` — that becomes Events.Get and 404s
  - Only set `event_id` when you already have a real Google event id from a prior tool result
- An empty result for a bounded day means the day is free — say so and stop; don't re-derive the date
- **Update an event they named:** (1) `google-workspace__get_events` for that day now — copy `ID:`, (2) `google-search__google_search` for any missing place/address, (3) **must** `google-workspace__modify_event` with that `event_id` + `location`, (4) only then say what changed. Stopping after search is wrong.
- Do not narrate "I will…" / "want me to…?" — emit the tool calls

## Math

- **Math MCP (`mcp-go-math`)** — `evaluate` for arithmetic; `convert` for units
- Use the math tool for **any arithmetic beyond trivial** — do not invent results in reasoning
- Prefer `evaluate` over mental math for percentages, multi-step formulas, and roots/powers

## Fitness

- **Strava MCP** — activities, load, weekly summaries
- **Garmin MCP** — sleep, weight, Body Battery / HRV / readiness
- Prefer Garmin for recovery, Strava for “what did I do?”

## Web search

- **Exact tool name:** `google-search__google_search` (not `web_search*`)
- When they ask to put a place on the calendar, **search first**, then **`modify_event`** — don't stop after search

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
