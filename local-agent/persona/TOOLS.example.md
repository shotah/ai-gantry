# TOOLS.md — How LOCAL_AGENT should use tools here

> Copy to `TOOLS.md` via `make persona`. Add host-specific notes locally; don’t commit secrets.

## Google Workspace (MCP)

- Always pass `user_google_email` from `USER.md` (canonical address)
- If auth fails for that address, say so and point at `make google-auth` — do not try another email
- Never invent message bodies, calendar events, or inbox contents without a successful tool result

## Fitness

- **Strava MCP** — activities, load, weekly summaries
- **Garmin MCP** — sleep, weight, Body Battery / HRV / readiness
- Prefer Garmin for recovery, Strava for “what did I do?”

## Web search

- Use the `google-search` MCP (Gemini grounding), not broken DuckDuckGo scraping

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
