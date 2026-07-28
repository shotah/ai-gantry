# MCP host

How gantry loads tools, names them for the model, recovers from common
hallucinations, and how to exercise that path locally.

Capabilities live in **external MCP stdio binaries**. The kernel only
supervises them: spawn → list → call → truncate → restart. See
[architecture.md](architecture.md) for the process diagram; this page is the
operator contract for naming and local use.

---

## Why MCP (and nothing else)

| Goal | How MCP helps |
| --- | --- |
| Keep the kernel small | Calendar, search, Cast, etc. stay out of `gantry` |
| Clear grant model | A server in `mcp.toml` is granted; omit it and it does not exist |
| Distroless-friendly | Static Go binaries over stdio — no shell, no npm in the image |
| Swappable brains | Same tool schemas work for Gemini, Grok, Ollama — OpenAI-compat tool calls |

Chat, memory, and cron work with **zero** MCP servers. Tools are optional.

---

## Naming: `{server}__{tool}`

Every published tool reaches the model as:

```text
{server_or_tools_prefix}__{original_tool_name}
```

Examples:

| Manifest `name` | MCP tool | Name the model must call |
| --- | --- | --- |
| `google-search` | `google_search` | `google-search__google_search` |
| `google-workspace` | `get_events` | `google-workspace__get_events` |
| `math` | `evaluate` | `math__evaluate` |

**Why the prefix?** OpenAI-safe characters, no collisions across servers, and
obvious provenance in logs / collapsed history markers.

Optional override in `mcp.toml`:

```toml
[[server]]
name = "garmin"
tools_prefix = "garm"   # tools become garm__get_sleep, …
```

Default prefix is the server `name`. Prefer short, stable prefixes when a
hyphenated product name fights the model (see below).

Inspect what the running agent sees:

- Telegram / Discord / Slack / stdio: `/tools`
- Boot logs: `mcp server connected` with `tools_listed` vs `tools_published`

---

## Local models and hyphenated prefixes

Server names often use hyphens (`google-search`). Tool *suffixes* often use
underscores (`google_search`). Small local models (e.g. Qwen via Ollama)
frequently **normalize the whole name to underscores** and invent nearby
names (`web_search`, `gmail_search`, …).

Typical failure spiral:

1. Model calls `google-search__web_search` → unknown tool  
2. Host suggests exact catalog: `google-search__google_search`  
3. Model “fixes” the prefix to `google_search__google_search` → unknown prefix  
4. Hint degrades to a bare prefix list → more guessing → think-stall

That is a **runtime** problem, not a persona typo. Persona `TOOLS.md` should
still spell exact names; the host also hardens the call path.

### Alias resolve (automatic)

On `Call`, if the exact name is missing, gantry rewrites **only the server
prefix** so underscores become hyphens, then retries lookup:

```text
google_search__google_search  →  google-search__google_search   ✅ called
google_workspace__get_events  →  google-workspace__get_events   ✅ called
```

Tool suffixes are **not** rewritten (`google_search` stays `google_search`).
When an alias fires, logs show:

```text
mcp tool name aliased  requested=…  resolved=…
```

### Unknown-tool suggestions

If lookup still fails, the error string is model-facing and catalog-aware:

| Mistake | Hint shape |
| --- | --- |
| Wrong tool, real prefix | `valid google-search tools are: google-search__google_search — retry with one of these exact names` |
| Underscored prefix, wrong tool | `did you mean "google-search"?` + that server’s exact tool list |
| Unknown prefix | `available server prefixes are: cast, garmin, google-search, …` |

The agent loop feeds that error back as a tool result so the next iteration
can self-correct (same pattern as argument/schema failures).

### What aliasing does *not* fix

- Invented tool suffixes (`…__web_search`) — still need a retry with the
  suggested name (or a tighter `TOOLS.md` / smaller tool surface)
- Wrong arguments (e.g. passing a time range as `event_id`) — MCP/API errors
- Think-only turns with no tool call — agent nudge / stall path in
  `internal/agent` (separate from naming)

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

### B — Personal-assistant compose (kernel + optional MCP)

```bash
make example-pa
# edit examples/personal-assistant/.env and mcp.toml
docker compose -f examples/personal-assistant/compose.yml up -d --build
```

MCP servers stay commented until you grant them. Same `/tools` / `/status`
commands on Telegram once the bot is up.

### C — local-agent appliance

Full life-stack (Workspace, search, Strava, …): see
[local-agent/README.md](../local-agent/README.md). Tool-specific setup:

| Capability | Guide |
| --- | --- |
| Web search | [local-agent/docs/web-search.md](../local-agent/docs/web-search.md) |
| Google Workspace | [local-agent/docs/google-workspace.md](../local-agent/docs/google-workspace.md) |
| Local / alternate chat models | [local-agent/docs/models.md](../local-agent/docs/models.md) |

For native Ollama + Qwen deploys, keep exact names in `persona/TOOLS.md` and
rely on alias + suggestions when the model mangles hyphens.

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
- [ ] Put exact call names in persona `TOOLS.md` (especially hyphenated prefixes)
- [ ] After deploy, `/tools` once and confirm the names you documented
- [ ] On weird tool loops: check logs for `aliased` vs repeated `unknown tool`
- [ ] Static MCP binaries only if you ship distroless (no libc/shell for children)

---

## Related

- [architecture.md](architecture.md) — host restart sequence
- [design.md](design.md) — principles / config contract
- [choices.md](choices.md) — why `{server}__{tool}` and tool-surface budget
- [security.md](security.md) — MCP child = trusted code
- Root [readme.md](../readme.md) §5.2 — manifest sketch
