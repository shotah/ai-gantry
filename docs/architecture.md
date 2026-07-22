# Architecture

ai-gantry is a single static Go binary that hosts one persona, one LLM
endpoint, and a set of MCP tool processes. Scaling is horizontal: one
container per brain.

## Container view

```mermaid
flowchart LR
  TG[Telegram API] <-->|long poll outbound| K

  subgraph Container["container — distroless/static-debian12:nonroot"]
    K[gantry]
    M1[mcp binary A]
    M2[mcp binary B]
    K -->|stdio MCP| M1
    K -->|stdio MCP| M2
  end

  K -->|HTTPS OpenAI-compat| LLM[one LLM endpoint]
  K --- P[("/persona/*.md ro")]
  K --- MF[("/etc/gantry/mcp.toml ro")]
  K --- D[("/data/gantry.db")]
  M1 --- S[("/secrets/... ro")]
```

Nothing listens inbound. Health is `gantry status` (exit code) reading a
heartbeat row in SQLite — Docker exec form, no shell.

## Package layout

```text
cmd/gantry/          run | status | version
cmd/release/         semver bump → tag → push (dev tooling)
internal/config/     env parse + fail-fast validation
internal/channel/    Channel interface; telegram/, stdio/
internal/provider/   OpenAI-compatible Completer (one implementation)
internal/mcp/        manifest, spawn, list/call tools, truncate, restart
internal/agent/      prompt assembly, tool loop, collapse, reply
internal/session/    bounded history + rolling summary
internal/memory/     Memory interface, builtin SQLite/FTS5, MCP adapter, consolidator
internal/persona/    load + concat /persona/*.md
internal/heartbeat/  singleton row for Docker healthcheck
internal/drain/      in-flight turn wait on SIGTERM
```

## Process model (goroutines)

One OS process. Concurrent work:

| Goroutine | Job |
| --- | --- |
| channel poller | Telegram `getUpdates` (or stdio REPL); allowlist filter |
| agent handler | per message: assemble → model → tools → reply (Telegram: workers=1) |
| MCP children | one OS process per manifest server (stdio), supervised by host |
| heartbeat ticker | upsert `heartbeat` every ~15s |
| memory consolidator | optional timer (`MEMORY_CONSOLIDATE_MINUTES`; `0` = off) |

```mermaid
flowchart TB
  subgraph gantry["gantry process"]
    CH[channel.Run]
    AG[agent.Handle]
    MCP[mcp.Host]
    HB[heartbeat.Start]
    CON[memory.Consolidator]
    CH -->|Handler| AG
    AG -->|Complete| LLM[(provider)]
    AG -->|Call| MCP
    AG -->|Append / Summary| SES[(session SQLite)]
    AG -->|Hydrate / tools| MEM[(memory)]
    HB --> DB[(gantry.db)]
    SES --> DB
    MEM --> DB
    CON --> MEM
    CON --> LLM
    MCP -->|stdio| C1[mcp child…]
  end
```

## Boot sequence

```mermaid
sequenceDiagram
  participant OS
  participant Run as gantry run
  participant Cfg as config
  participant Ses as session
  participant HB as heartbeat
  participant MCP as mcp.Host
  participant Mem as memory
  participant Ch as channel

  OS->>Run: start
  Run->>Cfg: Load env (fail-fast)
  Run->>Ses: Open gantry.db + migrations
  Run->>Ses: WithSummarizer(LLM)
  Run->>HB: OpenDB + Start ticker
  Run->>MCP: Start(manifest) — connect all servers or exit 1
  alt MEMORY_ENABLED
    Run->>Mem: OpenDB or MCPAdapter
    opt builtin + consolidate > 0
      Run->>Mem: Consolidator.Start
    end
  end
  Run->>Ch: Run(ctx, drain.Handler(agent.Handle))
  Note over Ch: blocks until SIGTERM / cancel
```

## Message / agent loop

```mermaid
sequenceDiagram
  participant U as User
  participant Ch as channel
  participant A as agent
  participant M as memory
  participant S as session
  participant L as LLM
  participant T as mcp / memory tools

  U->>Ch: inbound message
  Ch->>A: Handle(ctx, msg)
  alt /new or /status
    A->>S: Reset or Stats
    A-->>Ch: short reply
  else chat turn
    A->>S: Messages + Summary
    A->>M: Hydrate(query, ~30)
    A->>A: assemble system blocks + history + user
    loop until final text or TOOL_MAX_ITERATIONS
      A->>A: collapse tool results older than last 4
      A->>L: Complete(messages, tool schemas)
      alt tool_calls
        loop each call
          A->>T: Call(name, args)
          T-->>A: truncated result
        end
      else text
        A-->>Ch: reply
        A->>S: Append user+assistant (may trim → fold summary)
      end
    end
  end
  Ch-->>U: outbound reply
```

## MCP tool call (with restart)

```mermaid
sequenceDiagram
  participant A as agent
  participant H as mcp.Host
  participant C as child Conn

  A->>H: Call("server__tool", args)
  H->>C: CallTool(originalName, args)
  alt success
    C-->>H: text
    H-->>A: Truncate(text, TOOL_RESULT_MAX_CHARS)
  else failure
    H->>H: restartServer (backoff ≤ 4 attempts)
    H->>C: CallTool again
    C-->>H: text or error
    H-->>A: result / error string
  end
```

Children are **not** bound to the signal context. On SIGTERM the channel
stops accepting work, `drain.Gate` waits for the in-flight turn (default 2m),
then deferred `mcp.Host.Close()` tears down stdio sessions (killing children).

## Data on disk

One WAL SQLite file: `$DATA_DIR/gantry.db`.

| Table | Owner package | Purpose |
| --- | --- | --- |
| `session` / `session_message` | `session` | history + rolling `summary` |
| `memory` / `memory_fts` | `memory` | structured long-term memory |
| `heartbeat` | `heartbeat` | singleton row for `gantry status` |

`/new` deletes the session row (cascade messages + summary). Memory rows are
untouched.

## Prompt assembly (order)

1. System: persona markdown (+ memory persona-precedence note when memory on)
2. System: `[memory]` hydration block (optional, ≤ ~30 rows)
3. System: `[session summary]` (optional)
4. History: user/assistant turns (bounded)
5. User: current message

Tool schemas are attached on the completion request, not as chat messages.

## External dependencies (import over write)

| Concern | Library |
| --- | --- |
| MCP client | `modelcontextprotocol/go-sdk` |
| SQLite | `modernc.org/sqlite` |
| Telegram | `go-telegram/bot` |
| LLM | `openai/openai-go/v3` (custom base URL) |
| Env | `caarlos0/env/v11` |
| Manifest | `pelletier/go-toml/v2` |
| Logs | stdlib `log/slog` → **stderr** |

See [choices.md](choices.md) for why each pick stuck.

## Cron push (Milestone 6)

```mermaid
sequenceDiagram
  participant U as User
  participant A as agent
  participant C as cron
  participant T as Telegram

  U->>A: "remind me at 5pm to…"
  A->>C: cron_schedule(...)
  Note over C: SQLite job row + next_run
  C->>C: ticker: job due
  C->>A: Handle(synthetic prompt)
  A->>A: tools / MCP as usual
  A->>T: push SendMessage (no inbound update)
  T-->>U: reminder / digest
```

Outbound push needs a channel API beyond “reply to the update that invoked
Handle” — Telegram chat/user id is stored with the job from the scheduling turn.

## Planned: streaming (Milestone 7)

```mermaid
sequenceDiagram
  participant L as LLM stream
  participant A as agent
  participant T as Telegram

  A->>T: SendMessage("…")
  loop token chunks
    L-->>A: delta
    A->>T: editMessageText (throttled)
  end
```

Tool-call iterations can stay buffered; streaming targets the final assistant
text path first.
