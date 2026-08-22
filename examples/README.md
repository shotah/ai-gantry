# examples/

Three **template consumer repositories** for [ai-gantry](https://github.com/shotah/ai-gantry)
(plus cloud VM variants under [`hosting/`](hosting/)).
Each directory is shaped like a standalone git repo that **consumes** the published
**AI harness** (Hub image or release binary) — not a checkout of the harness itself.

Long-horizon planning (memory, cron, personality) ships in that binary. MCP
tools are optional grants you add in the consumer.

| Template | Consumes | Supervisor |
| --- | --- | --- |
| [`docker/`](docker/) → *gantry-compose* | `shotah/ai-gantry` image | Docker Compose |
| [`native/`](native/) → *gantry-native* | release binary | systemd |
| [`hosting/gcp/`](hosting/gcp/) → *gantry-gce* | Hub image on GCE | Compose + optional Actions |
| [`hosting/aws/`](hosting/aws/) → *gantry-ec2* | Hub image on EC2 | Compose + optional Actions |

Also here (harness scaffolding, not consumer templates):

| Path | Role |
| --- | --- |
| [`persona/PERSONA.example.md`](persona/) + [`SELF.example.md`](persona/SELF.example.md) | Two files; `gantry init` embeds them. Writing: [docs/persona.md](../docs/persona.md) |
| [`mcp.toml.example`](mcp.toml.example) / [`env.example`](env.example) | Same |

A full life-stack (persona + MCP + compose) lives in a consumer repo, not this harness.

```mermaid
flowchart LR
  K[ai-gantry harness · Hub / releases]
  K --> D[examples/docker]
  K --> N[examples/native]
  K --> HG[examples/hosting/gcp]
  K --> HA[examples/hosting/aws]
```

## Use as a separate repo

1. Copy one template directory to a new remote (or publish it as a GitHub template).
2. `make init` inside that tree — seeds `persona/*.md` and `.env` / `gantry.env`.
3. Set channel + LLM secrets; follow that template’s README.

Inside this monorepo, the same seed helpers exist from the harness root:

```bash
make example-docker
make example-native
make example-hosting-gcp
make example-hosting-aws
```

## Pick a template

| Goal | Template |
| --- | --- |
| Laptop / any Docker host, Gemini + Telegram | [`docker/`](docker/) |
| Linux mini-PC, Ollama, systemd | [`native/`](native/) |
| Always-on VM in an existing GCP project | [`hosting/gcp/`](hosting/gcp/) |
| Always-on VM in an existing AWS account | [`hosting/aws/`](hosting/aws/) |

All four share the harness contract: env + `persona/` + `mcp.toml` +
`$DATA_DIR/gantry.db`. No inbound app ports — chat channels dial out only.
Long-horizon state lives in that data dir and in `SELF.md`.
