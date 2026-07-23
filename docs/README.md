# ai-gantry docs

Operator-oriented design notes for the runtime. The [root readme](../readme.md)
is the product contract and milestone checklist; these pages go deeper.

| Doc | What it covers |
| --- | --- |
| [../todo.md](../todo.md) | Open follow-ups only |
| [milestones.md](milestones.md) | Archived M0–M7 build checklist (shipped) |
| [../examples/](../examples/) | Cookbook + appliance-style [`personal-assistant/`](../examples/personal-assistant/) compose |
| [../local-agent/](../local-agent/) | Full local-agent appliance (MCP tools + remote deploy + auth helpers) |
| [architecture.md](architecture.md) | Process model, packages, mermaid diagrams + sequences |
| [design.md](design.md) | Principles, agent loop, memory, config/ops contract |
| [security.md](security.md) | Threat model, tradeoffs, residual risks |
| [choices.md](choices.md) | Decision log (why we picked X over Y) |
| [memory.md](memory.md) | Hand-inspect / fix builtin SQLite memory with `sqlite3` |
| [cron.md](cron.md) | Schedule tools, timezone, inspect jobs, overlap policy |

Start with **architecture** if you're new to the code; **security** before
exposing a bot to a Telegram allowlist with real tool credentials.
