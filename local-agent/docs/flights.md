# Flights (Google Flights via SerpAPI)

gantry has no built-in flight search (by design — capabilities are MCP binaries).
We ship
[`flights-search-mcp`](https://github.com/shotah/flights-search-mcp)
— a **static Go** MCP that calls **SerpAPI’s Google Flights** engine.
Search + recommend + **checkout handoff**; it never buys tickets.

```mermaid
flowchart LR
  GN[gantry daemon] -->|MCP stdio| FL[flights-search-mcp]
  FL -->|engine=google_flights| S[SerpAPI]
  S --> GF[Google Flights data]
```

There is **no official Google Flights API**. SerpAPI wraps the consumer product
into stable JSON. Free tier is about **250 searches/month**.

Agent recipes live in `persona/TOOLS.md` (and `TOOLS.example.md`).

---

## Setup

1. Create a [SerpAPI](https://serpapi.com/) account and copy an API key.
2. Put it in `.env`:

```bash
SERPAPI_API_KEY=...
# Optional pin (Docker bake / native fetch):
# FLIGHTS_SEARCH_MCP_VERSION=v0.0.2
```

3. Rebuild / native-fetch and restart:

```bash
make build && make up
# or native: make remote-native-fetch / remote-native-deploy-dev
```

---

## Config wiring

`mcp.toml` (listed = granted):

```toml
[[server]]
name = "flights"
command = "flights-search-mcp"
download_tag = "latest"
download_url = "https://github.com/shotah/flights-search-mcp/releases/download/{tag}/flights-search-mcp_{version}_{os}_{arch}.tar.gz"
```

| Tool | Host name | Use |
| --- | --- | --- |
| `airports_search` | `flights__airports_search` | City → IATA |
| `dates_search` | `flights__dates_search` | Sample prices in a date window (≤7 calls) |
| `offers_search` | `flights__offers_search` | Fixed-date search + recommendation + tokens |
| `returns_search` | `flights__returns_search` | Round-trip: `departure_token` → return offers + `booking_token` |
| `booking_options_get` | `flights__booking_options_get` | **Real checkout** — `booking.url` / `post_data` |
| `link_format` | `flights__link_format` | Fallback *search* URL only (no API) |
| `account_get` | `flights__account_get` | SerpAPI quota left (free; no search burn) |

Tool names follow
[mcp-naming.md](../../docs/mcp-naming.md) — **no** `flights_offers_search`
(that would double-prefix as `flights__flights_offers_search`).

**Buy path:** one-way → `booking_options_get(booking_token)`; round-trip →
`returns_search(departure_token)` then `booking_options_get`. Present
`options[].booking` (POST when `post_data` is set). `link_format` /
`google_flights_url` are search fallbacks only — never invent `#flt=` links.

---

## Gov / contract travel (v1.6)

On `offers_search` / `dates_search`:

- `include_airlines` / `exclude_airlines` — IATA, alliances
  (`SKYTEAM` / `ONEWORLD` / `STAR_ALLIANCE`), or presets `alaska_partners` /
  `delta_partners`
- `exclude_basic` — skip basic economy (US domestic + economy only; ≠ refundable)
- `fly_america=annotate|filter` + `funding=civilian|dod` — marketing-designator
  **hint**, not legal certification

On `booking_options_get`:

- `refundable` per option (`yes` / `no` / `unknown` from Google text)
- `refundable_only=true` drops options labeled non-refundable

Cabin (`travel_class=economy`) is not the same as a refundable fare.

---

## Cost (ballpark)

SerpAPI free: **~250 searches / month**. Paid plans start around **$25 / 1,000**.
`dates_search` multiplies cost (one search per sample; capped at 7). Prefer
`offers_search` when dates are known. Booking hops also cost a search.
Cached identical queries are free on SerpAPI (1h cache). Search responses also
attach a `usage` snapshot.

---

## Smoke tests

```bash
make build
docker compose run --rm --entrypoint flights-search-mcp gantry --version
docker compose run --rm --entrypoint flights-search-mcp gantry --self-test
# Real search needs SERPAPI_API_KEY in the container env — try Telegram:
# “Find nonstop SEA to SFO next Friday under $200”
# Gov demo: “economy Alaska partners SEA→CDG with Fly America annotate + refundable checkout”
```

Native host: any `[[server]]` with `download_url` is fetched by
`gantry tools-plan` / `make remote-native-fetch`.

---

## Troubleshooting

| Symptom | Fix |
| --- | --- |
| LOCAL_AGENT doesn’t see flight tools | Check `[[server]]` in `mcp.toml`; rebuild so `flights-search-mcp` is in the image; `/tools` should list `flights__offers_search` |
| `SERPAPI_API_KEY is required` | Set key in `.env` and recreate the container |
| Image build fails in flights stage | Publish a release on [flights-search-mcp](https://github.com/shotah/flights-search-mcp/releases); or pin `FLIGHTS_SEARCH_MCP_VERSION` |
| Rate / quota errors | Slow down `dates_search`; check SerpAPI dashboard or `flights__account_get` |
| Stale binary missing checkout / Fly America | Bump / refetch `flights-search-mcp` release; restart daemon |
