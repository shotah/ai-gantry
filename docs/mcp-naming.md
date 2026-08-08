# MCP tool naming guidelines

Canonical contract for **shotah** MCP binaries consumed by ai-gantry (and any
host that prefixes `{server}__{tool}`). Sibling packages should link here from
their `TODO.md` / README rather than inventing a local dialect.

**Audience:** authors of google-mcp, go-garmin, go-strava-mcp, youtube-go-mcp,
mcp-beam, mcp-go-math, mcp-gemini-search, flights-search-mcp, rentals-search-mcp,
cars-search-mcp, and future MCPs.
**Why it matters:** small models (Qwen) and host closest-match repair pick tools
by **name tokens + description**. Synonyms and double prefixes break matching.

Related: host behavior ([mcp.md](mcp.md)), persona recipes (`TOOLS.md`).

---

## Layers (do not confuse them)

| Layer | Who owns it | Example |
| --- | --- | --- |
| MCP **server id** | Host `mcp.toml` `name` | `youtube`, `google`, `cast` |
| MCP **tool name** | Binary registration | `videos_search`, `calendar_list_events` |
| **Host-facing name** | Host joins with `__` | `youtube__videos_search` |

```text
{server_id}__{service}_{verb}_{object}[_{qualifier}]
```

---

## Hard rules

1. **Tool = `{service}_{verb}_{object…}`**
   Service first (resource / domain), then verb, then object. Same family as
   google-mcp (`calendar_list_events`, `gmail_search_messages`).

2. **Never put the server id in the tool name**
   Host already prefixes.
   - Bad: `youtube_videos_search` → `youtube__youtube_videos_search`
   - Bad: `strava_get_activities` → `strava__strava_get_activities`
   - Good: `videos_search` → `youtube__videos_search`

3. **No dual aliases**
   One name set per release. Breaking renames get a semver bump + host
   `TOOLS.md` update in the same change.

4. **Stable verb set** (prefer these; don’t invent synonyms)

   | Verb | Meaning |
   | --- | --- |
   | `list` | Collection / page / day window |
   | `get` | One resource by id |
   | `search` | Query → hits |
   | `create` / `update` / `delete` | Mutations |
   | `format` | Pure reshape / hint (no side effects) |

   Avoid verb-first tools (`search_tracks`, `get_sleep`) — rename to
   `tracks_search` / `sleep_get` style when touching that surface.

5. **Shared nouns across packages** when the concept is shared

   | Concept | Preferred service token | Used by |
   | --- | --- | --- |
   | Calendar events | `calendar_` | google |
   | Activities / workouts | `activities_` | garmin, strava |
   | Devices (LAN / cast) | `devices_` | cast (and garmin hardware — host prefix disambiguates) |
   | Playlists | `playlists_` | youtube |
   | Library shelves | `library_` | youtube |
   | Videos (YouTube Data API) | `videos_` | youtube (not `tracks_` — that was Music/InnerTube era) |
   | Cast handoff helpers | `cast_` | youtube (`cast_format_target`); beam owns playback tools |
   | Math | `expression_` / `units_` | math |
   | Web search | `web_` | google-search (`web_search`) |
   | Flight offers / dates / airports / returns / booking / quota | `offers_` / `dates_` / `airports_` / `returns_` / `booking_` / `link_` / `account_` | flights |
   | Rental listings / rent estimate / markets / areas / quota | `listings_` / `rent_` / `markets_` / `areas_` / `link_` / `account_` | rentals |
   | Car listings / VIN / markets / quota | `listings_` / `vin_` / `markets_` / `link_` / `account_` | cars (host prefix disambiguates from rentals) |

6. **Descriptions sell the intent**
   First sentence = what the agent wants (“Search YouTube videos…”, “List
   liked videos…”). Host nearest-name / Qwen fragment matching uses these
   tokens. Do not start with “Calls the YouTube Data API v3 Search.list…”

7. **Args: snake_case**
   `task_list_id`, `video_id`, `time_min` — never camelCase in schemas
   (`tasklistId`). Teach-in errors > silent defaults.

8. **Don’t steal another server’s domain words**
   No `calendar_*` on fitness MCPs for Google Calendar asks. No `mail_*` on
   Strava. If a product has a “calendar” of workouts, name it
   `training_calendar_*` (or exclude it from lean/core tiers).

9. **Lean catalogs for agent hosts**
   Prefer `--tool-tier core` / presets / `exclude` so SAM’s published set stays
   small. Fat schemas starve local models even when names are perfect.

10. **Tests lock names**
    Assert every registered tool matches `^[a-z]+_[a-z]+` and does **not**
    start with the server id string.

---

## Host forms (ai-gantry defaults)

| Server id (`mcp.toml`) | Example tools | Host calls |
| --- | --- | --- |
| `google` | `calendar_list_events` | `google__calendar_list_events` |
| `google-search` | `web_search` | `google-search__web_search` |
| `garmin` | `sleep_get`, `activities_list` | `garmin__sleep_get` |
| `strava` | `activities_list` | `strava__activities_list` |
| `youtube` | `videos_search`, `library_list_liked_videos` | `youtube__videos_search` |
| `cast` | `devices_list`, `youtube_beam_video` | `cast__devices_list` |
| `math` | `expression_evaluate`, `units_convert` | `math__expression_evaluate` |
| `flights` | `offers_search`, `dates_search`, `airports_search`, `returns_search`, `booking_options_get`, `link_format`, `account_get` | `flights__offers_search` |
| `rentals` | `listings_search`, `listings_get`, `rent_estimate_get`, `markets_get`, `areas_resolve`, `link_format`, `account_get` | `rentals__listings_search` |
| `cars` | `listings_search`, `listings_get`, `vin_get`, `markets_get`, `link_format`, `account_get` | `cars__listings_search` |

Hyphenated server ids (`google-search`) are fine; tool suffixes stay underscores.
The host aliases `google_search__web_search` → `google-search__web_search` on
call — **suffixes are never rewritten**.

---

## Checklist for a new or reshaped MCP

- [ ] Pick a **short** server id (`youtube`, not `youtube-music-go-mcp`)
- [ ] Every tool is `{service}_{verb}_{object…}` with no server-id prefix
- [ ] Diff verbs/nouns against this table + google-mcp before inventing a third synonym
- [ ] Descriptions lead with agent intent
- [ ] Args snake_case; hot paths documented in ai-gantry `TOOLS.md` when shipped
- [ ] Name assertion tests; no dual registration
- [ ] Release notes include old→new map; bump semver on break
- [ ] Update ai-gantry persona + docs in the same consumer PR

---

## Anti-patterns (seen in the wild)

| Anti-pattern | Why it hurts | Fix |
| --- | --- | --- |
| `strava_*` tools on server `strava` | Double prefix | Drop tool-level brand |
| `tracks_*` vs `videos_*` mixed | Qwen nearest-match thrash | One noun per product era |
| `search_tracks` (verb-first) | Breaks `{service}_{verb}_…` scanners | `videos_search` |
| `get_daily_schedule` as a memory “skill” name | Not a host tool name | Skills cite real `google__calendar_list_events` |
| Publishing `calendar_get` on Garmin | Steals Google Calendar | Rename or `exclude` |

---

## Where this lives

- **Canonical:** this file in [ai-gantry](https://github.com/shotah/ai-gantry)
- **Host mechanics:** [mcp.md](mcp.md)
- **Per-package TODOs:** link here; keep package-specific rename maps local
