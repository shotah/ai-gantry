# Event watches

A watch is a **cursor + poll**, not a chat loop. The harness calls an MCP fetch
tool on an interval. Quiet ticks never touch the model. New item ids wake the
same agent loop as cron, then **push** — or skip the push if the reply is
`[silent]`. This is long-horizon attention: notice the world later without
billing a Completer on every tick.

Do not implement this as `cron_schedule` + “fetch the feed; if nothing new,
`[silent]`.” That would spend a Completer call on every tick.

```text
ticker → Host.CallRaw(tool, args) → compare ids → empty? stop
                                 → new? agent.Handle → Push (or [silent])
```

The poller uses `CallRaw`, not `Call`. `TOOL_RESULT_MAX_CHARS` is for the model;
cutting a feed JSON mid-string makes `ParseItems` fail and the watch never seeds.

The first successful poll **seeds the cursor**. Old items are not dumped into
chat. Fetch tools must already be in the MCP manifest (`{server}__{tool}`).

## Config

Shares the cron ticker. No second poll interval.

| Env | Default | Meaning |
| --- | --- | --- |
| `WATCH_ENABLED` | `true` | Master switch |
| `WATCH_MAX` | `50` | Cap on enabled watches |
| `CRON_TICK_SECONDS` | `15` | How often the runner looks for due watches |

Boot fails if watch is on and the channel cannot `Push` (same as cron).

## Builtin tools

| Tool | Purpose |
| --- | --- |
| `watch_add` | Subscribe: prefixed MCP `tool` + `args` + `interval` (default `15m`, min `1m`) + optional `label` |
| `watch_list` | List active watches for this chat |
| `watch_cancel` | Disable by id from `watch_list` |

Example prompts once a fetch tool exists:

```text
Watch the NWS alert feed for Santa Clara and text me if something posts.
Stop watching NWS.
Watch @so-and-so on X and text me when they post.
```

## Fetch adapters

The poller does not know RSS vs Twitter. A watch row is `tool` + `args`.
Siblings return the same `{items:[{id,…}]}` JSON.

| Server | Binary | Tools | Auth | Watch args |
| --- | --- | --- | --- | --- |
| `feeds` | [feeds-mcp](https://github.com/shotah/feeds-mcp) | `items_list`, `source_resolve` | none (`FEEDS_USER_AGENT` optional for NWS) | `{ url }` |
| `twitter` | [twitter-mcp](https://github.com/shotah/twitter-mcp) | `posts_list` | `X_BEARER_TOKEN` on the gantry process (child inherits) | `{ handle }` |

```toml
[[server]]
name = "feeds"
command = "feeds-mcp"
download_tag = "latest"
download_url = "https://github.com/shotah/feeds-mcp/releases/download/{tag}/feeds-mcp_{version}_{os}_{arch}.tar.gz"
# tools = ["items_list", "source_resolve"]

[[server]]
name = "twitter"
command = "twitter-mcp"
download_tag = "latest"
download_url = "https://github.com/shotah/twitter-mcp/releases/download/{tag}/twitter-mcp_{version}_{os}_{arch}.tar.gz"
# tools = ["posts_list"]
```

Uncomment in [examples/mcp.toml.example](../examples/mcp.toml.example) (and the
docker / native / hosting copies). Put `X_BEARER_TOKEN` in `.env`, not in the
manifest. Prefer a **30–60m** interval for X (pay-per-use). Live-agent enablement
is a downstream consumer — not documented here.

## `[silent]`

Unchanged from cron. The wake still runs and is stored; the first line
`[silent]` drops the human-facing message. Use that when the new item is noise.

Prior `[watch]` / `[cron]` turns are omitted from the next scheduled prompt so
they cannot few-shot the next summary. Interactive chat still sees them.
