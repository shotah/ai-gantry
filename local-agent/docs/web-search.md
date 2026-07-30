# Web search (Gemini Google Search grounding)

gantry has no built-in web search (by design — capabilities are MCP binaries).
We ship
[`shotah/mcp-gemini-search`](https://github.com/shotah/mcp-gemini-search)
— a maintained **Go** fork (typed/compiled only; upstream `zchee` HEAD went
Python). Static binary `mcp-gemini-google-search` calls **Gemini Grounding with
Google Search** using the same `GEMINI_API_KEY` already in `.env`.

```mermaid
flowchart LR
  GN[gantry daemon] -->|MCP stdio| GS["mcp-gemini-google-search"]
  GS -->|generateContent + google_search| G[Gemini API]
  G --> Web[Google Search]
```

---

## Setup

Nothing extra beyond the Gemini key you already use for chat:

1. `.env` has `GEMINI_API_KEY` (paid / billing-enabled AI Studio project so
   grounding is allowed).
2. `GEMINI_MODEL` defaults to `gemini-3.5-flash` — chat **and** the search MCP
   both use it (the MCP reads `GEMINI_MODEL` / `GEMINI_API_KEY` from the
   container env).
3. Rebuild / native-fetch and restart:

```bash
make build && make up
# or native: make remote-native-fetch / remote-native-deploy-dev
```

Optional pin (Docker bake):

```bash
# GEMINI_SEARCH_MCP_VERSION=v0.0.1
```

---

## Config wiring

`mcp.toml` (listed = granted):

```toml
[[server]]
name = "google-search"
command = "mcp-gemini-google-search"
download_tag = "latest"
download_url = "https://github.com/shotah/mcp-gemini-search/releases/download/{tag}/mcp-gemini-google-search_{version}_{os}_{arch}.tar.gz"
```

Tool LOCAL_AGENT should use: `google-search__web_search` (query string).

**Why the hyphen matters:** the server prefix is `google-search`; the MCP tool
is `web_search`. Local models often call `google_search__web_search`
(underscored prefix) or invent `google_search` / `google-search__google_search`.
gantry aliases the underscored prefix automatically and suggests the exact
catalog on hard misses — see [../../docs/mcp.md](../../docs/mcp.md). Still put
the exact name in `persona/TOOLS.md`.

---

## Cost (ballpark)

Gemini 3 grounding (paid tier): ~**5,000 free grounded prompts / month**, then
about **$14 / 1,000 search queries**. Chat tokens are separate (already billed
via `GEMINI_API_KEY`). Free (no-billing) Gemini projects often cannot use
grounding.

---

## Smoke tests

```bash
make build
docker compose run --rm --entrypoint mcp-gemini-google-search gantry -version
# Binary is stdio-only; real check is Telegram:
```

Ask LOCAL_AGENT: “Search the web for today’s Seattle weather summary.”

He should call `google-search__web_search`.

---

## Troubleshooting

| Symptom | Likely fix |
|---|---|
| LOCAL_AGENT doesn’t see `web_search` | Check the `[[server]]` entry in `mcp.toml`; rebuild / re-fetch so the binary is present; Telegram `/tools` should list `google-search__web_search` |
| Logs show `unknown tool "google_search__…"` then success | Harmless if followed by `mcp tool name aliased` — host rewrote the prefix. If it keeps failing, the *suffix* is wrong (use `web_search`) |
| Logs show `unknown tool "…__google_search"` | Old tool name — use `google-search__web_search`. Tighten `persona/TOOLS.md`; see [../../docs/mcp.md](../../docs/mcp.md) |
| `GEMINI_API_KEY` / grounding errors | Billing enabled on the Google AI project; key has access to search grounding |
| Expensive search model | Keep `GEMINI_MODEL=gemini-3.5-flash` (MCP defaults to a Pro preview if unset) |
