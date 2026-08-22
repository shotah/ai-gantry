# AWS MCP — hunt

Attack this file. Harness loop work stays in [todo.md](todo.md).
Siblings: [apple_todo.md](apple_todo.md) · [ms_todo.md](ms_todo.md) ·
[gcp_todo.md](gcp_todo.md).

Prefix enable is why a fat AWS catalog can sit **off** until
`aws__billing`. That does **not** lift fit gate 3 — `npx` / `uvx` /
Python stay rejected. The list is “what daily question, what API,
write-or-import a static binary (Go / C / C++ / Rust).” Remote hosted
MCP also waits on **Outbound HTTP MCP** in `todo.md`.

---

**Need: BILLING** (then alarms). Not a cloud IDE. Official dump is
[awslabs/mcp](https://awslabs.github.io/mcp/servers) (~60 Python
servers) plus a managed remote
[AWS MCP Server (GA)](https://aws.amazon.com/blogs/aws/the-aws-mcp-server-is-now-generally-available/)
(~11 tools over 15k APIs, IAM + CloudTrail). Running gantry *on* EC2
is already [examples/hosting/aws](examples/hosting/aws) — different
item.

| Daily question | Look at | Runtime | Fit |
| --- | --- | --- | --- |
| What did this account cost? | Billing and Cost Management MCP; Pricing MCP | Python | **First cut.** Go SDK wrapper. |
| Is the box / alarm on fire? | CloudWatch MCP (metrics, alarms, logs); CloudTrail MCP | Python | Watch-tick fuel. |
| What does this service do? | Knowledge MCP (`knowledge-mcp.global.api.aws`, hosted); Documentation MCP | remote / Python | Needs outbound HTTP MCP. Search already answers a lot of this. |
| Just call the API | Managed AWS MCP Server; AWS API MCP (CLI-shaped) | remote / Python | Small tool count, huge blast radius. IAM is the ACL. |
| Places / geocode | Amazon Location Service MCP | Python | Skip — we already have `maps__*`. |

**Not doing from that catalog** (same reject as k8s / Grafana /
Paperless): EKS, ECS, Lambda-as-tools, SageMaker, Bedrock KB, every
RDS/Aurora/Dynamo/Redshift/Neptune flavor, Step Functions, IoT,
HealthOmics, Security Agent, IAM-as-a-toolbox, Well-Architected
scanners.

If a daily AWS question shows up, write a tiny Go binary for *that*
API (cost + CloudWatch alarms), or wait for outbound HTTP MCP and
point at the managed server with a locked-down IAM user. Do not bake
`uvx awslabs.*`.

When something ships: MCP page + `mcp.toml` snippet, then delete the
row here.
