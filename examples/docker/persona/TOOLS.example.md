# TOOLS.md — How to use tools here

> Copy to `TOOLS.md` via `make persona`. Add host-specific notes locally; don’t commit secrets.

**If a tool is in this file or in `/tools`, call it — do not claim it is absent.**
Wrong args → fix once and retry once; unknown tool → use the exact name below.
**Budget:** aim ≤6 tool calls/turn; stop by ~10 (`TOOL_MAX_ITERATIONS` default).

## Server routing (read first)

| Ask about… | Server | Never |
|---|---|---|
| Calendar / mail / tasks / docs / sheets | `google__…` | `strava__`, `garmin__` |
| Workouts / load | `strava__` / `garmin__` | `google__` |
| Sleep / recovery | `garmin__` | `strava__` for sleep |
| Saved Strava routes / `strava.app.link` | `strava__urls_resolve` / `strava__routes_*` | `maps__link_resolve` |
| Restaurants / “what’s near …” | `maps__place_search` | web search as the first hop |
| Maps share link / when do I leave | `maps__link_resolve` / `maps__route_eta` | `strava__routes_*` for leave-by |
| RSS / alerts / “watch this feed” | `feeds__…` + `watch_*` | inventing `twitter__*` |

Wrong-server error → switch prefix; don’t retry Strava/Garmin for calendar.
Never `garmin__calendar_*` for Google Calendar (Connect training calendar).

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
- **Calendar delete** (they named the event): `calendar_list_events` → id →
  `google__calendar_delete_event`. Do it — don’t invent “Google can’t delete.”
- **Gmail:** `google__gmail_search_messages` → `google__gmail_get_message`; send only if asked
  - “Today”: Gmail `after:YYYY/MM/DD` is *after* that day — for date D use
    `after:`(D−1) `before:`(D+1) with **slashes**, or `newer_than:1d`. Never
    `after:` today (skips today).
- **Tasks:** `task_list_id` (snake_case, not `tasklistId`); `google__tasks_list_tasks` / `google__tasks_create_task` (`task_list_id="@default"` or id from `tasks_list_tasklists`); new list → `google__tasks_create_tasklist` then create tasks in it
- **Docs / Sheets:** `google__docs_*` / `google__sheets_*` (everyday preset has no Drive tools)
- Do not narrate — call the exact `google__…` name from the tools list

## Math

- **Math MCP (`mcp-go-math`)** — `math__expression_evaluate` for arithmetic; `math__units_convert` for units
- Use the math tool for **any arithmetic beyond trivial** — do not invent results in reasoning
- Prefer `expression_evaluate` over mental math for percentages, multi-step formulas, and roots/powers

## Flights

- **Flights MCP (`flights-search-mcp`, server id `flights`)** — Google Flights via SerpAPI
- **Exact tools:** `flights__offers_search`, `flights__dates_search`, `flights__airports_search`, `flights__link_format`, `flights__account_get`
- City → `airports_search` first; recommend + `google_flights_url` — never claim a ticket was purchased

## Rentals

- **Rentals MCP (`rentals-search-mcp`, server id `rentals`)** — long-term apartments/houses via RentCast
- **Exact tools:** `rentals__listings_search`, `rentals__listings_get`, `rentals__areas_resolve`, `rentals__rent_estimate_get`, `rentals__markets_get`, `rentals__link_format`, `rentals__account_get`
- **Quota:** free tier is tight (~50/month). Check `rentals__account_get` / `usage` before burning calls. FREE: `areas_resolve`, `link_format`, `account_get`.
- **Thrifty:** ONE `listings_search` with combined `neighborhood=` or `zip_codes=` — never one call per area.
- Neighborhood → `areas_resolve` then `listings_search`; hand off listing URL — never apply or contact landlords; not for commercial leases

## Cars

- **Cars MCP (`cars-search-mcp`, server id `cars`)** — MarketCheck for-sale inventory
- **Exact tools:** `cars__listings_search`, `cars__listings_get`, `cars__vin_get`, `cars__markets_get`, `cars__link_format`, `cars__account_get`
- Zip/make/model search → hand off listing URL — never submit dealer leads; not `rentals__`

## Fitness

**Live tools only.** No sleep / recovery / activity numbers in the reply until a
tool this turn returned them. Chat memory and `memory_recall` do **not** count.

- **Strava MCP** — activities, load, weekly summaries (`strava__activities_list`, `strava__activities_get` / `_zones`, `strava__athlete_get_stats`). Saved courses: `strava__routes_list` / `strava__routes_get`. Share / short URL (`strava.app.link` or `strava.com/routes/…`): `strava__urls_resolve` first — do not use `maps__link_resolve` for Strava links.
- **Garmin MCP** (core) — `garmin__sleep_get`, `garmin__weight_get`, `garmin__wellness_get_body_battery`, `garmin__hrv_get`, `garmin__metrics_get_training_readiness`, `garmin__activities_list`, `garmin__activities_get`
- Prefer Garmin for recovery
- **“What did I do?” / yesterday’s ride / session:** call a list tool first — `garmin__activities_list` and/or `strava__activities_list` (whichever is in the tools list). Bound the day in the human’s timezone from `[current time]` (not UTC). Then by-id detail if needed. Do **not** ask the human to paste stats when a list tool exists; do **not** invent “no daily activity tool” because you only looked for `get_activities`.
- **Sleep:** `garmin__sleep_get` only — not Body Battery. Call it (don’t narrate “I’ll pull…”). For “last night” omit `date` or pass **today** (wake-up day). Example: today 2026-07-28 → `date=2026-07-28`, never yesterday

## Web search

- **Exact tool name:** `google-search__web_search` (not `google_search`, not `web_search_mcp_*`)
- New event + place → search if needed, then **`google__calendar_create_event`**. Existing + place → **`google__calendar_update_event`**
- If two cities share a gym-ish name, prefer the city in the ask / `USER.md`

## Feeds / watches

- **Feeds MCP (`feeds-mcp`, server id `feeds`)** — RSS / Atom / JSON Feed / NWS. No auth.
- **Exact tools:** `feeds__source_resolve` (site / page → feed URL), `feeds__items_list` (`url`)
- **Watch (kernel, not a chat loop):** `watch_add` with `tool=feeds__items_list`, `args` `{url}`, `interval` (default `15m`, min `1m`), optional `label`. First poll seeds the cursor — old items are not dumped into chat.
- `watch_list` / `watch_cancel` to manage. First line `[silent]` drops the human-facing push when the new item is noise.
- **X / Twitter is not granted** unless `X_BEARER_TOKEN` is set. Do not invent `twitter__*` tools.

## Maps

- **Maps MCP (`google-maps-mcp`, server id `maps`)** — share links, one place, nearby recommendations, route ETAs. Not `google-mcp` (that is Workspace OAuth).
- **Share / short URL first:** `maps__link_resolve` (`url`) for `maps.app.goo.gl`, `goo.gl/maps`, `g.co/maps`. No API key. Not for `strava.app.link`.
- **One place:** `maps__place_resolve` (`query`) — name, address, or share URL → coords, rating, a few reviews, Maps URL.
- **Restaurants / “what’s good near …”:** `maps__place_search` (`query`, optional `near`, `limit`). Do not invent a `places_search` synonym. Include the returned Maps links.
- **When do I leave / how far:** `maps__route_eta` (`origin`, `destination`, optional `mode`, `departure_time`). Modes: `driving` (default), `walking`, `bicycling`, `transit`. If the human bikes, pass `mode=bicycling` — do not assume driving. Always include the returned tap-to-open Maps URL. Not a Strava saved-route tool.
- Do not invent `google__maps_*` or put `maps` on the tool name.

## YouTube

- **YouTube MCP (`youtube-go-mcp` v1+, server id `youtube`)** — Data API v3 search, playlists, liked videos
- **Exact tools:** `youtube__videos_search`, `youtube__videos_get`, `youtube__library_list_playlists`, `youtube__playlists_get`, `youtube__library_list_liked_videos`, `youtube__cast_format_target`
- Optional `musicOnly=true` on search / liked for music-leaning results (not YouTube Music Liked Songs)
- Prefer this over inventing royalty-free / stock music URLs
- Returns `videoId` / `video_id` → hand off to Cast `cast__youtube_beam_video` (bare id, not a watch URL)
- Needs `make youtube-auth` (OAuth → `data/.config/youtube/oauth.json`)

## House Cast (speakers / displays)

- **Cast MCP (`mcp-beam`, server id `cast`)** — `cast__devices_list`, `cast__media_beam`, `cast__youtube_beam_video`, `cast__media_get_status`, play/pause/seek/stop/volume/mute
- Prefer Cast tools over shell hacks for speakers/TVs
- **Video/music flow:** YouTube MCP → pick `videoId` → `cast__youtube_beam_video` + room device — never invent free-MP3 fallbacks
- **Never** pass YouTube watch URLs to `cast__media_beam` (Nest connects, silence)
- Match the human’s **room name** to a local room→device map (fill in below after `make persona`), then `cast__devices_list` and pick the best-matching device `id`
- **Discovery defaults** (always pass these — slower Nest hubs can lose the race vs Mini/TV):
  - `timeout_ms`: **10000**
  - `include_unreachable`: **true**
  - If a known room device is missing, call `cast__devices_list` again a few seconds later (background mDNS cache), then map by room
- Volume: `cast__media_set_volume` (0–100) / `cast__media_mute` on an active session

### Room → devices (edit for your house)

| Room | Devices | Default target |
|---|---|---|
| Bathroom | … | … |
| Kitchen | … | … |
| Living room | … | … |
| Bedroom | … | … |

## Memory tools

- `memory_recall` — helpful, but **not** authoritative for the human’s email/name;
  **never** for live fitness/calendar/mail values
- `memory_store` — confirmed facts **and** `skill/<area>` tool craft (see `RULES.md` Skills); never a new identity for the human; never raw mail/calendar dumps; never metric snapshots
- `memory_forget` — delete contradictions with `USER.md` / obsolete skills
- Before a fiddly tool area: `memory_recall` query `skill/` — reuse your own recipes

## Self-notes (`self_note`)

- **`self_note` APPENDS one `-` line to `SELF.md` — it does not overwrite or distill.**
- Read the `SELF.md` section already in this prompt first. If the vibe/joke/ritual
  is already there, **do not call** — paraphrases are still duplicates.
- One short line only (voice, humor, running jokes, rituals). Not facts about the
  human (`memory_store`) and not tool recipes (`TOOLS.md` / `skill/`).
- Do this **unprompted** when a vibe lands. Full merge/dedupe of `SELF.md`
  happens only on `/new` — never mid-chat via this tool.

## Shell

- Prefer MCP/domain tools over shell hacking
- No destructive commands without asking
