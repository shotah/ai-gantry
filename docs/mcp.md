# MCP host

How the harness loads tools, names them for the model, recovers from common
hallucinations, and how to exercise that path locally.

Capabilities live in **external MCP stdio binaries**. The harness only
supervises them: spawn → list → call → truncate → restart. See
[architecture.md](architecture.md) for the process diagram; this page is the
operator contract for naming and local use.

Long-horizon planning uses tools over days (cron, watches). The host has to
repair names and finish turns or the horizon collapses into ERROR.

---

## Why MCP (and nothing else)

| Goal | How MCP helps |
| --- | --- |
| Keep the harness small | Calendar, search, Cast, etc. stay out of `gantry` |
| Clear grant model | A server in `mcp.toml` is granted; omit it and it does not exist |
| Distroless-friendly | Static Go binaries over stdio — no shell, no npm in the image |
| Swappable brains | Same tool schemas work for Gemini, ChatGPT, Ollama — OpenAI-compat tool calls |

Chat, memory, and cron work with **zero** MCP servers. Tools are optional.

---

## Naming: `{server}__{tool}`

**Authoring new MCP tools?** Follow the shared package contract:
[mcp-naming.md](mcp-naming.md) (`{service}_{verb}_{object}`, no server-id in the
tool name, stable verbs, sibling nouns). This section is the **host** side.

Every published tool reaches the model as:

```text
{server_or_tools_prefix}__{original_tool_name}
```

Examples:

| Manifest `name` | MCP tool | Name the model must call |
| --- | --- | --- |
| `google-search` | `web_search` | `google-search__web_search` |
| `google` | `calendar_list_events` | `google__calendar_list_events` |
| `math` | `expression_evaluate` | `math__expression_evaluate` |
| `youtube` | `videos_search` | `youtube__videos_search` |
| `cast` | `youtube_beam_video` | `cast__youtube_beam_video` |
| `garmin` | `sleep_get` | `garmin__sleep_get` |
| `strava` | `activities_list`, `urls_resolve` | `strava__activities_list` |
| `feeds` | `items_list` | `feeds__items_list` |
| `twitter` | `posts_list` | `twitter__posts_list` |
| `maps` | `place_search`, `route_eta` | `maps__place_search` |
| `boards` | `challenges_list` | `boards__challenges_list` |

**Why the prefix?** OpenAI-safe characters, no collisions across servers, and
obvious provenance in logs / collapsed history markers.

Optional override in `mcp.toml`:

```toml
[[server]]
name = "garmin"
tools_prefix = "garm"   # tools become garm__sleep_get, …
```

Default prefix is the server `name`. Prefer short, stable prefixes when a
hyphenated product name fights the model (see below).

Inspect what the running agent sees:

- Telegram / Discord / Slack / stdio: `/tools` (total + per-server `schema_est_tokens`)
- `/status` includes `schema_est_tokens` alongside history size
- Boot logs: `mcp server connected` (`tools_listed` vs `tools_published`), then
  `tool schema estimate` + one `tool schema by server` line per prefix
- **Fail-soft boot:** if one `[[server]]` fails to spawn/initialize (missing API
  key, broken binary, EOF), gantry logs `mcp server boot skipped` with a stable
  `reason` (`no_binary` / `no_key` / `no_oauth` / `connect`) and continues.
  Calling a skipped prefix returns `tool error [<reason>]: … is skipped — do
  not invent <name>__* names`. A single optional tool must not take down the
  agent. Operator JSON: [gantree-contract.md](gantree-contract.md).

---

## Local models and hyphenated prefixes

Server names often use hyphens (`google-search`). Tool *suffixes* use
underscores (`web_search`). Small local models (e.g. Qwen via Ollama)
frequently **normalize the whole name to underscores** and invent nearby
names (`google_search`, `gmail_search`, …).

Typical failure spiral:

1. Model calls `google-search__google_search` → unknown tool (old / invented suffix)  
2. Host suggests exact catalog: `google-search__web_search`  
3. Model “fixes” the prefix to `google_search__web_search` → unknown prefix  
4. Hint degrades to a bare prefix list → more guessing → think-stall

That is a **runtime** problem, not a persona typo. Exact names come from the
live catalog (tool schemas + `[mcp prefixes]` + `/tools`); the host also
hardens the call path.

### Alias resolve (automatic)

On `Call`, if the exact name is missing, gantry rewrites **only the server
prefix** so underscores become hyphens, then retries lookup:

```text
google_search__web_search  →  google-search__web_search   ✅ called
```

Tool suffixes are **not** rewritten (`web_search` stays `web_search`).

A real tool name wearing an invented or missing prefix is repaired the same way,
because a bounced call costs a full model round-trip — the most expensive thing
in a local-model turn:

```text
mcp__hrv_get  →  garmin__hrv_get   ✅ called
hrv_get       →  garmin__hrv_get   ✅ called
```

Two cases are left to the model on purpose:

- **Real prefix, wrong tool** (`garmin__athlete_get` when only Strava has it).
  The model chose that server deliberately, so it gets that server's catalog
  rather than a silent hop to a different server.
- **Two servers publish the name** (`activities_get` on both Garmin and Strava).
  Guessing is a coin flip, so both real names go back as a hint.

When any repair fires, logs show:

```text
mcp tool name aliased  requested=…  resolved=…
```

### Unknown-tool suggestions

If lookup still fails, the error string is model-facing and catalog-aware:

| Mistake | Hint shape |
| --- | --- |
| Wrong tool, real prefix | `valid google-search tools are: google-search__web_search — retry with one of these exact names` |
| Underscored prefix, wrong tool | `did you mean "google-search"?` + that server’s exact tool list |
| Invented name (fake prefix, merged suffix) | `closest real names are: garmin__wellness_get_body_battery, garmin__hrv_get — retry with one of these exact names` |
| Unknown prefix, nothing close | `available server prefixes are: cast, garmin, google-search, …` |

The agent loop feeds that error back as a tool result so the next iteration
can self-correct (same pattern as argument/schema failures).

Closest-name ranking scores shared name tokens, ignoring generic verbs (`get`,
`list`, `set`, …) that would otherwise match everything. A model that invents a
tool tends to stitch real fragments together — `mcp__get_hrv_and_body_battery`
is `get_hrv` plus `get_body_battery` under a prefix that does not exist — so the
fragments identify what it was reaching for even when neither prefix nor suffix
is real.

### Printed tool calls

A model can also skip the `tool_calls` field entirely and *print* the call:

```json
{ "name": "garmin__get_daily_activity", "parameters": { "date": "2026-07-28" } }
```

Without handling, that JSON is just assistant text, so it becomes the visible
reply — the agent answering in wire format. gantry parses it back into a real
call and runs it, accepting the object bare, fenced, inside `<tool_call>` tags,
or embedded in prose, with arguments under `arguments`, `parameters`, `args`, or
`input` (models pick all of them).

```text
model printed a tool call instead of emitting one; executing it  name=… chars=90
```

The name must look like a tool — published, or at least carrying a `server__`
prefix — so an ordinary reply that happens to contain JSON is never hijacked. An
unpublished but prefixed name is still run on purpose: the host's answer is what
names the real tools, which feeds the retry below.

### Grammar-constrained retry

A hint only works if the model reads it. When a name cannot be resolved *and*
there are real candidates, the next model call is constrained instead: gantry
sends `response_format` with a JSON schema whose `name` is an `enum` of those
candidates. Ollama compiles that to a GBNF grammar and masks every token that
would spell anything else, so a bad name stops being unlikely and becomes
impossible.

```text
tool call failed        name=mcp__get_hrv_and_body_battery
constraining retry …    requested=mcp__get_hrv_and_body_battery candidates=2
model call              iteration=2 forced_tool_names=2
```

Three details make it work:

- **`tools` stays in the request.** The model reads real parameter schemas from
  there. Drop it and the name is still legal but the arguments are invented
  (measured on Qwen: `start_date`/`end_date` instead of `date`).
- **The call arrives in `content`, not `tool_calls`.** Ollama omits `tool_calls`
  whenever a `response_format` is set (still true in 0.32.4), so the provider
  parses the JSON object back into a `ToolCall`.
- **It is one-shot, and never streams.** A grammar forces *every* reply to be
  JSON, so leaving it on would make conversational answers impossible; and
  streaming it would type raw JSON into the user's bubble.

No candidates means no constraint — forcing a call out of the whole catalog is
just a different guess.

### What aliasing does *not* fix

- Invented tool suffixes (`…__web_search`) — no real name to repair to, so these
  still need a retry with the suggested name (or a smaller published tool surface)
- Wrong arguments (e.g. passing a time range as `event_id`) — MCP/API errors
- Think-only turns with no tool call — agent nudge / stall path in
  `internal/agent` (separate from naming)
- A model that neither calls nor prints anything callable — still a nudge, then
  the turn answers with whatever prose it has

---

## Manifest filters

Listed servers **start**. `tools` / `exclude` only filter what is **published**
to the model:

```toml
[[server]]
name = "garmin"
command = "garmin"
args = ["mcp"]
tools = ["get_sleep", "get_weight", "get_hrv"]  # allowlist
# exclude = ["raw_*"]
```

Boot logs `tools_listed` vs `tools_published`. Schema cost is estimated as
`est_tokens` (chars/4); `TOOL_SCHEMA_MAX_TOKENS` can hard-fail an oversized set.
Prefer MCP-native tiers (`--tool-tier core`) first — see [choices.md](choices.md).

### Prefix enable (`dynamic_tools`)

By default (`dynamic_tools` omitted or `true`) MCP schemas stay **off** until
the agent calls `mcp_enable` (list of prefixes, next Completer call in the
same turn). The prompt lists on vs off under `[mcp prefixes]` and tells the
model to review that list and enable a needed off prefix this turn. Brief hold
idles out at 6h (morning/afternoon); short at 27h. Harness builtins stay on.
Go-live is from zero — no seed of today's catalog.

Small models / rollback — full catalog every turn, no `mcp_enable`:

```toml
dynamic_tools = false
```

Furniture that should never idle-drop while dynamic tools are on:

```toml
[[server]]
name = "google"
force = true          # whole server prefix; pair with a tight `tools` allowlist
```

Or `MCP_ENABLE_FORCE=google__calendar,garmin__sleep`. Human overrides:
`/brief` `/short` `/off`. `/tools` shows published vs available.

---

## Using this locally

### A — REPL (fastest feedback)

```bash
# from repo root
make init          # deploy/persona + deploy/mcp.toml
# edit deploy/mcp.toml — uncomment / add servers you have binaries for
# edit .env — LLM_* (Gemini, Ollama, …)

make run           # CHANNEL=stdio by default
```

In the REPL:

```text
/status     # model, history, tool count
/tools      # exact prefixed catalog the model sees
```

Ask something that needs a tool (“search for …”, “what’s on my calendar
today”). Watch stderr JSON for `tool call`, `mcp tool name aliased`, or
`tool call failed` with the suggestion string.

Override mounts:

```bash
# Windows PowerShell example
$env:CHANNEL="stdio"
$env:PERSONA_DIR="./deploy/persona"
$env:MCP_MANIFEST="./deploy/mcp.toml"
make run
```

### B — Personal-assistant compose (harness + optional MCP)

```bash
make example-docker
cd examples/docker   # set .env + mcp.toml, then:
make up
```

MCP servers stay commented until you grant them. Same `/tools` / `/status`
commands on Telegram once the bot is up.

For native Ollama + Qwen deploys, trust `/tools` + alias/suggestions when the
model mangles hyphens. Do not copy the MCP catalog into `PERSONA.md`
([persona.md](persona.md)).

### Unit tests (no LLM)

```bash
go test ./internal/mcp/ -count=1
```

Covers catalog suggestions, underscore-prefix aliasing, and “did you mean”
hints without spawning real MCP binaries.

---

## Operator checklist

- [ ] Only list servers this persona should have (`mcp.toml` = grant)
- [ ] Prefer MCP `--tool-tier` / `tools = […]` so Flash/local models see tens of tools, not hundreds
- [ ] After deploy, `/tools` once and confirm the published names
- [ ] On weird tool loops: check logs for `aliased` vs repeated `unknown tool`
- [ ] Static MCP binaries only if you ship distroless (no libc/shell for children)

---

## Related

- [architecture.md](architecture.md) — host restart sequence
- [design.md](design.md) — env contract + MCP manifest sketch
- [persona.md](persona.md) — `PERSONA.md` is not the tool catalog
- [choices.md](choices.md) — why `{server}__{tool}` and tool-surface budget
- [security.md](security.md) — MCP child = trusted code
- [watch.md](watch.md) — poller + `feeds-mcp` / `twitter-mcp` / `boards-mcp`
