# Design

## Problem

ZeroClaw-style runtimes drift toward multi-agent platforms (many providers,
dashboards, config UIs). Our deployment model is the opposite:

```text
container = persona + model + MCP set + memory volume
```

Want another LLM or persona? Another container. ai-gantry is the kernel that
does exactly that — nothing else.

## Principles

1. **Stupid simple.** One agent, one model, one channel loop. If it needs a
   diagram to explain, it probably belongs in an MCP binary.
2. **Highly performant.** Pure Go, static binary, no CGO, small RSS. Long-poll
   + goroutines; nothing dials in.
3. **Highly portable.** `CGO_ENABLED=0`, distroless/static final image. No
   glibc in our binary; no writable rootfs beyond mounts.
4. **Plugin-centric.** Capabilities are external MCP stdio binaries. The gantry
   hosts tools; it does not implement them (except three builtin memory tools).
5. **1:1, always.** No multi-provider config, no peer routing. Scale = compose.
6. **Env/compose is the config plane.** Secrets/scalars via env. Structure via
   mounts: persona markdown, MCP manifest, data volume.
7. **Memory is structured and inspectable.** SQLite rows + FTS5, not opaque
   embedding blobs. Persona files always outrank recalled memory.

## Non-goals

- Web dashboard, gateway, REST/WS API, pairing flows
- Multi-agent / multi-provider / model fallback chains
- Channels beyond Telegram + stdio (interface exists; others don't ship)
- Built-in search/workspace tools (those are MCP binaries)
- Vector DB / embedding service
- In-process sandboxing / risk profiles (the container is the sandbox;
  Telegram allowlist is the gate)

## Planned (post daily-drive)

| Milestone | Feature |
| --- | --- |
| **6** | Cron / scheduled turns — shipped; see [cron.md](cron.md) |
| **7** | Streaming replies — provider chunks → Telegram `editMessageText` (optional stdio stream) |

See [root readme §11](../readme.md#11-todo--build-order) for checklists.

## Configuration contract

Everything is env or a mount. Boot is fail-fast: missing required env → exit 1.

### Environment (summary)

| Area | Vars |
| --- | --- |
| LLM | `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL` (required) |
| Channel | `CHANNEL` (`telegram`\|`stdio`); Telegram token + allowlist when telegram |
| Mounts | `PERSONA_DIR`, `DATA_DIR`, `MCP_MANIFEST` |
| Bounds | `HISTORY_MAX_MESSAGES`, `HISTORY_MAX_TOKENS`, `TOOL_RESULT_MAX_CHARS`, `TOOL_MAX_ITERATIONS` |
| Memory | `MEMORY_ENABLED`, `MEMORY_BACKEND` (`builtin`\|`mcp:<name>`), `MEMORY_CONSOLIDATE_MINUTES` |
| Cron | `CRON_ENABLED`, `CRON_TZ`, `CRON_MAX_JOBS`, `CRON_TICK_SECONDS` |
| Ops | `LOG_LEVEL` |

Full table lives in the [root readme](../readme.md#51-environment-variables).

### MCP manifest

The one structured file (lists of processes don't fit env). Mounted read-only.
If a server is listed, the agent gets its tools — the container composition
**is** the grant. Tool names are always `{server}__{tool}`.

### Persona

All `*.md` under `PERSONA_DIR`, concatenated in a fixed order. Missing files are
tolerated; empty persona is allowed but unusual.

## Agent loop & context bounds

Boring on purpose:

1. Assemble prompt (persona → memory → summary → history → user).
2. Call model with eager-loaded tool schemas.
3. Execute tool calls; truncate each result; loop until final text or
   `TOOL_MAX_ITERATIONS`.
4. Reply on the channel; append the turn.

**Bounds that keep prompts finite:**

| Mechanism | Behavior |
| --- | --- |
| History caps | Drop oldest past `HISTORY_MAX_MESSAGES` / `HISTORY_MAX_TOKENS` (chars/4 **estimate**) |
| Rolling summary | Trimmed turns fold into `session.summary` via the same LLM; reinjected later |
| Tool truncate | Each MCP/memory tool result capped at `TOOL_RESULT_MAX_CHARS` |
| Tool collapse | Tool payloads older than the last 4 become one-line markers |
| Iteration cap | Hard stop at `TOOL_MAX_ITERATIONS` |

`/new` clears session history + summary. Memory is untouched.

## Memory design

Direction: structured rows + timer consolidation (no vectors). At personal
scale, FTS5 + kinds beat ANN and stay greppable. See [memory.md](memory.md)
for hand inspection.

### Builtin tools (only non-MCP tools)

| Tool | Role |
| --- | --- |
| `memory_store` | Atomic `kind` / `subject` / `content` |
| `memory_recall` | FTS5 + recency |
| `memory_forget` | By id or query — memory must be correctable |

Auto-save is **off**. The model stores deliberately; the consolidator promotes
episodes → durable facts/preferences/people/insights.

### Backend switch

`MEMORY_BACKEND=builtin` (default) or `mcp:<server>` routing the same three
tools + hydration through a manifest server. Builtin consolidator only runs
for the SQLite backend.

### Persona precedence

When memory is enabled, the system prompt states that `/persona` outranks
recalled rows. Contradictions should be surfaced to the user, not obeyed.

## Ops surface

| Command / signal | Behavior |
| --- | --- |
| `gantry run` | Daemon (default) |
| `gantry status` | Exit 0 if heartbeat fresh (≤ ~60s); used by Docker healthcheck |
| `gantry version` | Build ldflags |
| SIGTERM / Interrupt | Stop channel → drain in-flight turn → close MCP → close DB |
| Logs | JSON `slog` on stderr (`docker logs`) |
| Chat cmds | `/new`, `/status` |

No port is opened by the gantry, ever.

## Packaging

- Static binary (`CGO_ENABLED=0`), GoReleaser on `v*` tags (`make release`)
- Image: multi-stage → `gcr.io/distroless/static-debian12:nonroot`
- MCP children must also be static (no libc/shell in the final image)
- Healthcheck: exec form `["CMD","/usr/local/bin/gantry","status"]` only

## Related

- [architecture.md](architecture.md) — diagrams and sequences
- [security.md](security.md) — threats and tradeoffs
- [choices.md](choices.md) — decision log
