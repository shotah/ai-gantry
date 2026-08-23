# ai-gantry docs

Operator notes for the **AI harness**. The [root readme](../readme.md) is the
public pitch and Docker hello path. Harness contract (env, loop, memory,
long-horizon planning) lives in [design.md](design.md); diagrams in
[architecture.md](architecture.md). What we actually shipped, ranked:
[features.md](features.md).

| Doc | What it covers |
| --- | --- |
| [features.md](features.md) | **Inventory:** The Great / The Good / The Okay / The Ugly |
| [persona.md](persona.md) | **Write `PERSONA.md`:** two files, tight budget, no MCP catalog, north-star vs memory vs cron |
| [gantree.md](gantree.md) | Yard console (sibling): [shotah/gantree](https://github.com/shotah/gantree) — metrics, grants, several agents |
| [gantree-contract.md](gantree-contract.md) | File/env/`gantry status` JSON the console may write and read |
| [positioning.md](positioning.md) | ICP, competition, Docker Hub story, when (not) to add a site |
| [dockerhub.md](dockerhub.md) | **Hub overview** synced by CI (keep lean; PNG banner) |
| [deploy-docker.md](deploy-docker.md) | **Fastest hello:** Hub pull + compose; MCP auth (chat `/auth` or laptop) |
| [auth.md](auth.md) | **Chat `/auth`** headless OAuth (PKCE paste + device flow); catch page |
| [deploy-native.md](deploy-native.md) | Linux systemd + Ollama/Qwen, local-model hardening |
| [observability.md](observability.md) | Memory/GPU/timing commands: `ollama ps`, per-turn log recipes, `docker stats` |
| [mcp.md](mcp.md) | MCP host: `{server}__{tool}` naming, underscore alias, suggestions, local REPL |
| [mcp-naming.md](mcp-naming.md) | **Shared** tool naming contract for all shotah MCP packages (Qwen / closest-match) |
| [discord.md](discord.md) | Discord Gateway channel (DMs, allowlist, no ports) |
| [slack.md](slack.md) | Slack Socket Mode channel (DMs + @mention, no ports) |
| [milestones.md](milestones.md) | Archived M0–M7 build checklist (shipped) |
| [../examples/](../examples/) | Consumer templates: [`docker/`](../examples/docker/) · [`native/`](../examples/native/) · [`hosting/`](../examples/hosting/) ([`gcp/`](../examples/hosting/gcp/) · [`aws/`](../examples/hosting/aws/)) |
| [architecture.md](architecture.md) | Process model, packages, mermaid diagrams + sequences |
| [design.md](design.md) | Principles, harness + long-horizon, agent loop, memory, config/ops contract |
| [security.md](security.md) | Threat model, tradeoffs, residual risks |
| [choices.md](choices.md) | Decision log (why we picked X over Y) |
| [memory.md](memory.md) | Hand-inspect / fix builtin SQLite memory with `sqlite3` |
| [troubleshooting.md](troubleshooting.md) | **`SELF.md` audit/prune**, self-notes `:ro` disable, `/new` personality, tool budget |
| [cron.md](cron.md) | Schedule tools, timezone, inspect jobs, overlap policy |
| [watch.md](watch.md) | Poll MCP fetch tools; `feeds-mcp` / `twitter-mcp` adapters |

Start with **deploy-docker** (Hub pull) or **deploy-native** for a running bot;
**features** for the honest inventory;
**[gantree](https://github.com/shotah/gantree)** for the yard console
(not this harness); **auth** for headless MCP login;
**positioning** for who/why; **architecture** for the code; **mcp** before
wiring tools; **security** before exposing an allowlist with real tool
credentials. Audit grown personality in **troubleshooting** (`SELF.md`)
whenever the vibe drifts. Keep `PERSONA.md` short ([persona.md](persona.md)).
Long-horizon planning is the goal; the pages above are how the harness holds it.
