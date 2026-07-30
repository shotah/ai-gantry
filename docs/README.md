# ai-gantry docs

Operator-oriented design notes for the runtime. The [root readme](../readme.md)
is the product contract and milestone checklist; these pages go deeper.

| Doc | What it covers |
| --- | --- |
| [../todo.md](../todo.md) | Open follow-ups only |
| [deploy-native.md](deploy-native.md) | **Featured:** Linux systemd + Ollama/Qwen, local-model hardening |
| [deploy-docker.md](deploy-docker.md) | Distroless compose / Hub, Gemini hello path |
| [mcp.md](mcp.md) | MCP host: `{server}__{tool}` naming, underscore alias, suggestions, local REPL |
| [mcp-naming.md](mcp-naming.md) | **Shared** tool naming contract for all shotah MCP packages (Qwen / closest-match) |
| [discord.md](discord.md) | Discord Gateway channel (DMs, allowlist, no ports) |
| [slack.md](slack.md) | Slack Socket Mode channel (DMs + @mention, no ports) |
| [milestones.md](milestones.md) | Archived M0–M7 build checklist (shipped) |
| [../examples/](../examples/) | Cookbook + appliance-style [`personal-assistant/`](../examples/personal-assistant/) compose |
| [../local-agent/](../local-agent/) | Full local-agent appliance (MCP tools + remote deploy + auth helpers) |
| [architecture.md](architecture.md) | Process model, packages, mermaid diagrams + sequences |
| [design.md](design.md) | Principles, agent loop, memory, config/ops contract |
| [security.md](security.md) | Threat model, tradeoffs, residual risks |
| [choices.md](choices.md) | Decision log (why we picked X over Y) |
| [memory.md](memory.md) | Hand-inspect / fix builtin SQLite memory with `sqlite3` |
| [cron.md](cron.md) | Schedule tools, timezone, inspect jobs, overlap policy |

Start with **deploy-native** or **deploy-docker** for a running bot;
**architecture** for the code; **mcp** before wiring tools; **security** before
exposing an allowlist with real tool credentials.
