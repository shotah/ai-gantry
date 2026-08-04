# ai-gantry docs

Operator-oriented design notes for the runtime. The [root readme](../readme.md)
is the public pitch + product contract; these pages go deeper.

| Doc | What it covers |
| --- | --- |
| [positioning.md](positioning.md) | ICP, competition, Docker Hub story, when (not) to add a site |
| [dockerhub.md](dockerhub.md) | **Hub overview** synced by CI (keep lean; PNG banner) |
| [deploy-docker.md](deploy-docker.md) | **Fastest hello:** Hub pull + compose; **MCP browser auth** (Google/Strava) |
| [deploy-native.md](deploy-native.md) | Linux systemd + Ollama/Qwen, local-model hardening |
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

Start with **deploy-docker** (Hub pull) or **deploy-native** for a running bot;
**positioning** for who/why; **architecture** for the code; **mcp** before
wiring tools; **security** before exposing an allowlist with real tool
credentials.
