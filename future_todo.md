# ai-gantry — future todo

**Features and tooling to add.** The agent already runs. Deleted todos stay
deleted — they were non-starters, not a backlog. This is not a lockdown /
hardening / “safe the agent” list.

- Shipped: [docs/milestones.md](docs/milestones.md)
- Locked decisions: [docs/choices.md](docs/choices.md)

Status: **next** · **later** · **maybe**  
Size: **S** ≈ an afternoon · **M** ≈ a weekend

---

## Fit gates

1. **Outbound by default.** Prefer poll / long-poll / outbound WebSocket
   (what Telegram/Slack already are). An inbound port is discussable if we
   can say why poll is not enough **and** how it stays locked down (not
   `0.0.0.0` + hope). See [Event watches](#event-watches--something-happened).
2. **MCP or nothing.** Kernel work only for things a tool server cannot do
   (channel, loop, memory, cron, and a watch wake).
3. **Native compiled binary. No JIT.** Go / C / C++ / Rust. Never Python,
   Node, `npx`, `uvx`, Bun, JVM. Things we author stay **Go**
   (`CGO_ENABLED=0`).
4. **Static child** `gantry tools-fetch` can pin. Distroless has no runtime.
5. **Import over write.**
6. **Tiny catalog.** Every tool schema is re-billed every turn.
7. **Still 1:1.** One persona, one model, one channel.

---

## Tooling — maps

Calendar answers “what’s next.” Maps answers **“when do I leave?”** That is
the gap. Reuse the existing Google OAuth story (`google-mcp` already has the
client). Prefer a **google-mcp preset / a few route tools** over a second
binary if the APIs fit; otherwise a small `shotah/maps-mcp`.

| | |
| --- | --- |
| Tools (target ≤4) | `place_resolve`, `route_eta`, maybe `route_steps` |
| Ask | “I have climbing at 6 — when do I need to leave?” |
| Status | **next** · M |

---

## Features (only if they expand what he can do)

| Feature | Why it is kernel / channel work | Status |
| --- | --- | --- |
| **Voice notes in** | Telegram voice → OpenAI-compat `/v1/audio/transcriptions` (`whisper.cpp` locally). Same tagged-text path as photos. | **later** · M |
| **Outbound HTTP MCP** | Host only speaks stdio today. Unlocks Home Assistant (and other HTTP MCP) without opening a port. No proxy sidecar. | **maybe** · M — only if we actually want HA |

---

## Event watches — “something happened”

The hole is not another digest at 5pm. Cron already does clock → agent loop →
**Push** (or `[silent]`). The hole is **an external event** → same loop →
call a tool and/or message you.

```text
ticker  →  poller (code)  →  empty? stop
                         ↘  new items?  →  agent.Handle  →  Push
```

No new chat channel. No dashboard. The agent is **not** the poller.

### What we gain

| Without a watch | With a watch |
| --- | --- |
| You remember to ask, or a daily cron guesses | He texts when it actually changes |
| Search every hour (tokens + “I checked”) | Fetch, diff, stay quiet if nothing is new |
| GitHub / weather / traffic as fat tool catalogs | One URL or one endpoint per subscription |

A quiet tick never calls the model. `[silent]` is for turns that *did*
run (a live-data cron digest with nothing to say). Polls should not get
that far.

### How a user talks to it

Subscribe (stored, not a one-shot):

- “Watch the NWS alert feed for Santa Clara and text me if something posts.”
- “Ping me when `shotah/ai-gantry` cuts a release.”
- “Follow this traffic-alert feed on the commute corridor.”
- “Watch this blog / newsletter feed.”

On demand (no watch):

- “What’s new on that feed since yesterday?”

Unsubscribe: “stop watching NWS” → `watch_cancel` / watch row gone.

Subscribe is a one-time agent turn (he writes a watch row). After that,
ticks are code. On-demand “what’s new?” is a normal chat turn with a
fetch tool — you asked, so tokens are expected.

### Poller is code, not the agent

The expensive design: cron every 15m with prompt “fetch the feed; if
nothing new, `[silent]`.” That is a full Completer call (persona +
schemas + history) **96 times a day per watch**, almost always to
decide nothing happened.

The design we want: a **codified poller**.

1. SQLite `watch` row: `url`, `interval`, `etag`, `seen_ids`, `session`.
2. Kernel ticker (can share `CRON_TICK_SECONDS`). Due? GET the URL
   (`If-None-Match` / `If-Modified-Since`). Parse items in Go (or a
   tiny compiled fetch helper — still not the LLM).
3. **304, unchanged etag, or zero new ids → return.** No
   `agent.Handle`. No tokens.
4. New items → *then* one synthetic turn: the new items only, as
   untrusted text. He may Push, call maps/calendar, or `[silent]` if
   the item is noise.

```text
every 15m, 4 watches, all quiet   →  4 HTTP GETs, 0 model calls
one NWS alert posts               →  1 model call, then Push
```

Do **not** implement watches as `cron_schedule` + a tool-using prompt.
Cron remains for clock jobs you already have (“5pm calendar digest”).
Watches are a cursor + HTTP, same as a mail fetcher, not a chat loop.

### Poll vs webhook vs “push”

| Transport | Direction | Latency | Fits today | When to use |
| --- | --- | --- | --- | --- |
| **Poll** | Kernel ticker dials out; agent only if the diff is non-empty | Minutes | **Yes** | RSS/Atom, NWS, GitHub `releases.atom`, any HTTP GET we can cursor |
| **Outbound push** | We dial a WebSocket / long-poll (Telegram already) | Seconds | Only if the vendor has this | Rare for feeds; don’t invent a gateway |
| **Webhook** | Something POSTs **to us** | Seconds | **Needs a listen port** | Sources with no poll/RSS and we care about seconds (CI failed, door opened, package out for delivery) |

**Ship poll first.** It is the RSS story, it needs no port, and it reuses
cron. Webhook is the inbound-port conversation: only if a source we
actually want cannot be polled.

### Feeds (RSS / Atom) — what they actually are

RSS was not killed. It is still the boring subscribe API for “this URL
published an item”: blogs, many newsrooms, GitHub releases, YouTube
channels, podcasts, **NWS alerts (Atom/CAP)**, and a lot of city/DOT
**traffic alert** feeds. It is not a general web crawl. If there is no
feed URL, this tool cannot help — use search or a dedicated API.

**RSS has no hook.** The protocol is GET the URL, parse items, remember
ids. WebSub (PubSubHubbub) exists on paper; almost no news/NWS/DOT feed
you would actually watch implements it. No poller means no RSS watches.
A webhook is only a thing when the *vendor* POSTs at you (GitHub, some
X products). That is a different source class.

| Feed type | Example ask | Source shape |
| --- | --- | --- |
| News / blogs | “Watch The Verge / this climbing blog” | Site `rss.xml` / `atom.xml` |
| Weather alerts | “Text me if NWS posts for my zone” | NWS Atom / CAP (keyless) |
| Traffic | “Alert me if 101 / I-5 has a new incident feed item” | DOT / 511 RSS (coverage varies) |
| GitHub (narrow) | “When this repo releases” | `github.com/{owner}/{repo}/releases.atom` — **not** a full github-mcp catalog |
| YouTube | “New upload on this channel” | Channel Atom (already have YouTube MCP for search; a watch is the subscribe half) |
| **X (Twitter)** | “Text me when @so-and-so posts” | **Not RSS.** Official profile RSS is gone. Needs an X API fetch adapter — see below. |

**Subscribe how:** one chat turn. “Watch NWS for Santa Clara” → he
resolves a URL (search or `source_resolve`) → builtin `watch_add` writes
the row. You are not paying for that URL again until something publishes.

**On-demand how:** “What’s new on that feed?” → agent calls `items_list`
once, like mail search. Not a watch tick.

### X / Twitter — same watch, different fetch, yes auth

“Watch this person on X” fits the watch runner. It does **not** fit
`feeds-mcp`. There is no `x.com/{user}/rss`. Nitter-style scrapers die
whenever X changes the site — we do not build on that.

Official path: X API v2 `GET /2/users/:id/tweets` (public posts).

| Question | Answer |
| --- | --- |
| Auth? | **Yes.** A developer app at `console.x.com` and a credential on every GET. |
| Which credential? | **App-only Bearer token** is enough for *public* accounts you do not log in as. No user OAuth, no `/auth x` browser dance. Env like `X_BEARER_TOKEN` — same shape as SerpAPI / search keys. |
| When is user OAuth needed? | Their **home timeline**, protected accounts, DMs, or posting. That is a different product (and our `/auth` flow). Skip unless we actually want “my For You.” |
| Free? | Not in any useful way (2026). Reads are pay-per-use. A 15-minute poll of N accounts is N timeline reads an hour, billed even when nobody posted. Poll slower (30–60m) or accept the meter. |
| On-demand without a watch? | “What did @foo just say?” can stay **web search**. A watch is only worth it if you want a Push when they post, without asking. |

Do not scrape. Do not pretend X is RSS. Ship URL/RSS watches first; add
`twitter-mcp` when someone will put a Bearer token in env and pay the reads.

**Kernel vs MCP — pick (this is the design):**

The poller is a **dispatcher**. It does not know RSS or Twitter. A watch
row names a tool + args; the poller `Host.Call`s it, diffs ids, maybe
wakes the agent.

| Job | Who | Why |
| --- | --- | --- |
| Fetch one source → list of `{id, title, url, …}` | **MCP** | Capability. Chat and poller use the same tool. |
| Store watches, tick, cursor, **only then** `Handle` + `Push` | **Kernel** | An MCP child cannot run the agent or outbound to Telegram ([choices.md](docs/choices.md#scheduled--cron-turns)) |

```text
watch {
  tool = "feeds__items_list"        args = { url = "…" }      // keyless HTTP
  tool = "twitter__posts_list"      args = { handle = "foo" } // token stays in twitter-mcp
  seen_ids, interval, session
}

ticker → Host.Call(tool, args) → compare ids → empty? stop
                                 → new? agent.Handle → Push
```

| Adapter | Binary | Auth | Poller args |
| --- | --- | --- | --- |
| HTTP / RSS / Atom / NWS | `feeds-mcp` | none | `{ url }` |
| X profile posts | `twitter-mcp` | `X_BEARER_TOKEN` on that process only | `{ handle }` |

The kernel never sees the Bearer token. `twitter-mcp` reads it from env
(manifest `env = ["X_BEARER_TOKEN"]`), same as Strava/search. Chat can
call `twitter__posts_list` on demand; the poller calls it on a tick
without the LLM.

Contract for any future adapter: return items with stable `id`s. That is
enough for the cursor. No new kernel code for Bluesky / YouTube Data /
etc. — another compiled MCP + a watch row.

**Not the poller:**

- **Not `feeds-mcp` as a long-running poller.** Stdio MCP is request/response. The server cannot `Handle` or `Push`. To “notify” gantry it would need a new host protocol (server-initiated MCP notifications) or a port. That is a worse kernel than a ticker we already know how to write.
- **Not a cron job.** `cron.Runner` always calls `Handle` (see `JobUserPrefix`). A `KindWatch` that sometimes skips the model turns every cron path into “does this job talk?” Cron stays clock → agent. Watches stay cursor → maybe agent. Two tables, same idea (SQLite + builtins + a ticker). They can share the process’s existing tick goroutine so we do not invent a second clock.

**Quiet tick cost:** one stdio tool call to a child that is already running (MCP supervisor). No Completer. That is the point.

**On-demand** still goes through the model because you asked: chat →
`feeds__items_list` or `twitter__posts_list` → reply.

**Subscribe** is one model turn: he resolves a URL, calls builtin `watch_add`, done. After that he is not in the loop until the cursor moves.

### Inbound ports — when we would even talk

Poll is minutes. That is fine for a blog and for NWS. It is late for
“CI went red” or “the lock just opened.”

A listen port is justified only if:

1. The source has a webhook (or MQTT, etc.) and **no** decent poll/RSS, and
2. Waiting a poll interval actually loses the moment, and
3. The socket is locked down.

Lock-down bar (or we don’t open it):

- Bind **Tailscale / localhost**, not public `0.0.0.0`.
- HMAC or shared secret on every POST (vendor signing, not “security by
  obscure path”).
- Allowlist source IPs if the vendor publishes them.
- Body is **untrusted text** — same as a forwarded Telegram message. The
  wake creates a synthetic turn (“GitHub: workflow failed on main”); the
  model still has to choose tools. The webhook does not execute tools
  itself.
- Fail closed: bad signature → 401, no turn.

That can be a few lines in the kernel (queue a cron-like job) or a tiny
static helper that only accepts signed POSTs and writes a row gantry
already ticks. No Python/Node sidecar. No dashboard.

GitHub in this frame is **one event source** (release / Actions failure),
not “give him issues, PRs, and the kitchen sink.” Full `github-mcp` stays
out unless we are living in that catalog every day.

### Build order

| Step | What | Where |
| --- | --- | --- |
| 1 | **Shipped.** Kernel poller: `watch` table, ticker, `watch_add` / `watch_list` / `watch_cancel`, `Host.Call` + cursor, wake only on new ids. `[silent]` still skips Push. | this repo |
| 2 | `feeds-mcp` — `items_list` / `source_resolve`, static Go, stdio, GoReleaser | sibling `/home/christopher/feeds-mcp` (see TODO.md) |
| 3 | `twitter-mcp` — `posts_list`, Bearer token on that child, same item shape | sibling `/home/christopher/twitter-mcp` (see TODO.md) |
| 4 | Wire both into gantry docs + `mcp.toml` examples (`download_url`, env, `tools` / `exclude`) | this repo |

Live-agent enablement is a downstream consumer. Not documented here.

Webhook inbound stays **maybe**, after this lands, and only for a source we cannot poll.

---

## Not doing

Deleted todos stay gone. Also:

| Idea | Why not |
| --- | --- |
| **Full github-mcp catalog** | He is not a dev assistant. A **release / Actions watch** is an event source (above), not 50 GitHub tools. |
| **filesystem MCP** | Docs/Drive already exist. A vault is a second store. |
| **weather / net-probe / nest / shipments / sky / mealie** | Search, Cast, Tasks, and calendar already cover the asks. New binary needs a daily question we cannot answer today. |
| **Paperless / Grafana / k8s** | Wrong persona. Fat catalogs. |
| **Python / Node / JIT MCP** | Anywhere. Write or import a native binary. |
| **Agent lockdown** (confirm-before-send, untrusted-forward wrappers, …) | He has been running fine. Do not invent a second ACL. |
| **Schema slimming** | Loved the token idea. Cannot ship it. The bytes you’d cut *are* the tool manual we wrote (descriptions, examples, comments, titles). A “lossless” pass that leaves those alone is just `$schema` / `$id` URIs — a micro cut. A pass that removes the manual damages call quality, which is the opposite of why that text exists. Token lever that does not touch the manual: publish fewer tools (`tools` / `exclude` / `--tool-tier`). Revisit only if a large cut appears that is not the agent-facing docs. |

---

## When something ships

Docs in the same change: env in [readme §4.1](readme.md#41-environment-variables),
MCP page + `mcp.toml` snippet with `tools` / `exclude`. Then delete the row
here.
