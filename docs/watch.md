# Event watches

A watch is a **cursor + poll**, not a chat loop. The kernel calls an MCP fetch
tool on an interval. Quiet ticks never touch the model. New item ids wake the
same agent loop as cron, then **push** — or skip the push if the reply is
`[silent]`.

Do not implement this as `cron_schedule` + “fetch the feed; if nothing new,
`[silent]`.” That would spend a Completer call on every tick.

```text
ticker → Host.Call(tool, args) → compare ids → empty? stop
                              → new? agent.Handle → Push (or [silent])
```

The first successful poll **seeds the cursor**. Old items are not dumped into
chat. Fetch tools must already be in the MCP manifest (`{server}__{tool}`).
This repo does not ship a feed or Twitter binary — those are sibling packages.

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
```

## `[silent]`

Unchanged from cron. The wake still runs and is stored; the first line
`[silent]` drops the human-facing message. Use that when the new item is noise.

Prior `[watch]` / `[cron]` turns are omitted from the next scheduled prompt so
they cannot few-shot the next summary. Interactive chat still sees them.
