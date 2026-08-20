# TOOLS.md — How to use tools here

> Copy to `TOOLS.md` via `make persona`. Add host-specific notes locally; don’t commit secrets.

**If a tool is in this file or in `/tools`, call it — do not claim it is absent.**
Wrong args → fix and retry; unknown tool → use the exact name below.

## Server routing

| Ask about… | Server | Never |
|---|---|---|
| Calendar / mail / tasks / docs / sheets | `google__…` | `strava__`, `garmin__` |
| Workouts / load | `strava__` / `garmin__` | `google__` |
| Sleep / recovery | `garmin__` | `strava__` for sleep |

Wrong-server error → switch prefix. Never `garmin__calendar_*` for Google Calendar.

## Timezone (all tools)

- Human timezone is in `USER.md` / `[current time]` (`CRON_TZ`). Match that; not UTC.
- Date-only args: local calendar day from `[current time]`.
- RFC3339: include that zone’s offset. **Never default to `Z`** unless asked or the schema requires it.

## Google (`google`)

`google__{service}_{verb}_…` (e.g. `google__calendar_list_events`). Not fitness.

- Always pass `user_google_email` from `USER.md`. Auth fails → `gantry auth google` — don’t try another email.
- Never invent mail/calendar/docs/sheets/tasks without a successful tool result.
- **Create:** `google__calendar_create_event` (not `calendar_update_event` — that needs `event_id`).
- **Day:** `google__calendar_list_events` — omit `event_id`; `calendar_id="primary"`, both `time_min` + `time_max`.
- **Update:** list → id → `google__calendar_update_event`.
- **Gmail:** `google__gmail_search_messages` → `google__gmail_get_message`; send only if asked.
  “Today”: `after:YYYY/MM/DD` is *after* that day — for date D use `after:`(D−1) `before:`(D+1) with **slashes**, or `newer_than:1d`. Never `after:` today.
- **Tasks:** `task_list_id` (snake_case); `@default` or id from `tasks_list_tasklists`. New list → `google__tasks_create_tasklist` then create tasks.
- **Docs / Sheets:** `google__docs_*` / `google__sheets_*` (everyday preset has no Drive).

## Math

`math__expression_evaluate` / `math__units_convert` for anything beyond trivial — don’t invent results in reasoning.

## Fitness

- **Strava:** `strava__activities_list`, `strava__activities_get` / `_zones`, `strava__athlete_get_stats`
- **Garmin (core):** `garmin__sleep_get`, `garmin__weight_get`, `garmin__wellness_get_body_battery`, `garmin__hrv_get`, `garmin__metrics_get_training_readiness`, `garmin__activities_list`, `garmin__activities_get`
- Prefer Garmin for recovery. Sleep: `garmin__sleep_get` only — “last night” omit `date` or pass **today** (wake-up day).
- “What did I do?”: list tool first (`garmin__activities_list` and/or `strava__activities_list`), bound the day in the human’s timezone, then by-id if needed. Don’t ask them to paste stats; don’t invent “no daily activity tool.”

## Web search

Exact name: `google-search__web_search`. New event + place → search if needed, then `google__calendar_create_event`. Existing + place → `google__calendar_update_event`.

## YouTube

`youtube__videos_search`, `youtube__videos_get`, `youtube__library_list_playlists`, `youtube__playlists_get`, `youtube__library_list_liked_videos`, `youtube__cast_format_target`. Optional `musicOnly=true`. Hand `videoId` to `cast__youtube_beam_video` (bare id, not a watch URL). Auth: `gantry auth youtube`.

## House Cast

`cast__devices_list`, `cast__media_beam`, `cast__youtube_beam_video`, `cast__media_get_status`, play/pause/seek/stop/volume/mute.

- YouTube → `videoId` → `cast__youtube_beam_video` + room device. **Never** pass watch URLs to `cast__media_beam`.
- Map room name → device `id` via `cast__devices_list`. Always pass `timeout_ms=10000`, `include_unreachable=true`. Missing device → list again a few seconds later.
- Volume: `cast__media_set_volume` (0–100) / `cast__media_mute`.

### Room → devices (edit for your house)

| Room | Devices | Default target |
|---|---|---|
| Bathroom | … | … |
| Kitchen | … | … |
| Living room | … | … |
| Bedroom | … | … |

## Memory

- `memory_recall` — not authoritative for email/name
- `memory_store` — confirmed facts and `skill/<area>` craft (see `RULES.md`); never a new identity or raw dumps
- `memory_forget` — contradictions with `USER.md` / obsolete skills
- Before a fiddly area: `memory_recall` query `skill/`

## Shell

Prefer MCP over shell. No destructive commands without asking.
