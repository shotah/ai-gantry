# Choices (decision log)

Short record of locked decisions. Product narrative stays in
[design.md](design.md); this is the “why not the alternative” list.

## Naming

**Pick:** repo `ai-gantry`, binary `gantry`.

A gantry holds and positions tools; the tools do the work. Earlier names lost
to collisions (`noclaw` in claw-benchmark lineage, `armature` as a Python YAML
harness).

## One provider implementation

**Pick:** OpenAI-compatible client only (`openai-go` + custom `base_URL`).

Gemini, Grok, Ollama, etc. already speak the shape. Model identity is
`LLM_BASE_URL` + `LLM_MODEL` + `LLM_API_KEY`. A provider registry would be
multi-agent platform gravity we don't want.

## Gemini 3 thought signatures

**Pick:** preserve `extra_content.google.thought_signature` on assistant
`tool_calls` when echoing them back; synthesize
`skip_thought_signature_validator` when streaming deltas omit it (v0.0.3+).

Gemini 3 OpenAI-compat returns a signature on each tool call and **requires**
it on the follow-up. Dropping it → HTTP 400 mid-loop (“something went wrong”
on Telegram). Other OpenAI-compat backends ignore the field. This is not
optional if Gemini 3 + tools is a supported path.

## Tool surface budget

**Pick:** prefer MCP-native filters (`--tools`, `--tool-tier`) plus optional
gantry `tools` / `exclude` in `mcp.toml`. Cap schemas via
`TOOL_SCHEMA_MAX_TOKENS` as a backstop.

Flash models degrade when fed ~150 tool schemas. Curating to tens of tools
is the largest latency/quality win after the thought-signature fix.

## Token counting

**Pick:** chars/4 **estimates**, labeled (`est_tokens`, “estimated”).

Avoids tokenizer deps and model-specific quirks. Revisit only if trim
misbehavior is proven in production.

## Memory: builtin + swappable

**Pick:** SQLite/FTS5 behind a `Memory` interface; `MEMORY_BACKEND=builtin|mcp:<name>`.

Builtin default for zero-deps personal use. MCP escape hatch lets people plug
experiments without forking the agent loop. Surface is exactly the three tools
+ hydrate — not a kitchen-sink plugin API.

**Rejected for v1:** vector DB / embeddings service (cost, privacy, ops) —
schema can grow an `embedding` blob later behind the same recall interface.

## Memory auto-save

**Pick:** off.

Auto-saved hallucinations (wrong emails, invented prefs) hurt more than
missing recall. Model stores deliberately; consolidator promotes episodes.

## Channel auth

**Pick:** allowlist only on every chat channel; empty allowlist fails boot
(`TELEGRAM_ALLOWED_USERS` / `DISCORD_ALLOWED_USERS` / `SLACK_ALLOWED_USERS`).

No pairing codes / interactive bind. That flow was an operational pain
elsewhere and fights “env is the config plane.”

## Runtime image

**Pick:** `gcr.io/distroless/static-debian12:nonroot`, not Alpine.

No shell, minimal surface, static binary contract. MCP children must be static
too. Healthchecks must use exec form, never `CMD-SHELL`.

## Logs on stderr

**Pick:** JSON `slog` → stderr.

Keeps stdout clean for `CHANNEL=stdio` REPL; Docker still captures both
streams.

## Streaming replies to the user

**Pick:** Milestone 7 — shipped, opt-in via `STREAM_REPLIES=true` (default off).

Streaming *to Telegram* is channel-layer work (send placeholder, throttled
`editMessageText`). MCP streaming tool results into the gantry is a different
problem. Cron pushes stay buffered (no ReplyWriter on that path).

## Scheduled / cron turns

**Pick:** Milestone 6 — builtin scheduler in the kernel (SQLite jobs + tools),
not a pure-MCP cron.

Firing a job must run `agent.Handle` and **push** on Telegram. An MCP server
alone cannot outbound to the channel. External `docker exec` poke remains a
valid interim escape hatch but is not the product surface.

## Tool naming

**Pick:** always `{server}__{tool}`.

OpenAI-safe characters, no collisions across servers, obvious provenance in
logs and collapse markers.

## Config plane

**Pick:** env + three mounts (persona, manifest, data). No config UI / `config set`.

Lists of processes don't fit env → one TOML manifest. Everything else fails
closed at boot.

## Healthcheck

**Pick:** SQLite heartbeat row + `gantry status` exit code.

Avoids opening a port “just for k8s/docker.” Proves process liveness + DB
writability, not end-to-end Telegram/LLM health (accepted limitation — see
[security.md](security.md)).

## Graceful shutdown

**Pick:** drain in-flight handler; MCP children not tied to signal context.

`CommandContext(signalCtx)` would kill tools mid-turn on SIGTERM. Children die
on `Host.Close()` after drain instead.

## Release process

**Pick:** same as other shotah MCP repos — `make release` + GoReleaser on `v*`
tags, pre-commit via `make install-hooks`.

Consistency beats inventing a second release culture.

## Channels

**Pick:** Telegram (default) + Discord + Slack + stdio in-tree.

All three messengers are outbound-only + allowlist (no inbound ports). Slack uses
**Socket Mode only** (HTTP Events API is a non-goal). Signal is next — see
[todo.md](../todo.md). Setup: [discord.md](discord.md), [slack.md](slack.md).

## Related

- [design.md](design.md)
- [architecture.md](architecture.md)
- [security.md](security.md)
