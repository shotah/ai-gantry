# GCP MCP — hunt

Attack this file. Kernel loop work stays in [todo.md](todo.md).
Siblings: [apple_todo.md](apple_todo.md) · [ms_todo.md](ms_todo.md) ·
[aws_todo.md](aws_todo.md).

Prefix enable is why a fat Cloud catalog can sit **off** until
`gcp__billing`. That does **not** lift fit gate 3 — `npx` / `uvx` /
Python stay rejected. The list is “what daily question, what API,
write-or-import a static binary (Go / C / C++ / Rust).” Remote hosted
MCP also waits on **Outbound HTTP MCP** in `todo.md`.

---

**Need: BILLING.** Prompt-token estimates are not an invoice
([docs/observability.md](docs/observability.md)). First cut is Cloud
Billing (enable the API, `billing.readonly`, `/auth`). Not Monitoring.

Not a cloud IDE. Official dump is
[50+ Google-managed remote MCP servers](https://docs.cloud.google.com/mcp/supported-products)
(`https://SERVICE.googleapis.com/mcp`) plus local
[googleapis/gcloud-mcp](https://github.com/googleapis/gcloud-mcp)
(`npx @google-cloud/gcloud-mcp` → one `run_gcloud_command` tool).
Running gantry *on* GCE is already
[examples/hosting/gcp](examples/hosting/gcp) — different item.

**Not this file:** Gmail / Calendar / Tasks / Docs. That is
`google-mcp` (already shipped). Google’s own remote Workspace MCPs
(`gmailmcp` / `calendarmcp` / Drive / Chat / People) are a later
swap for that binary, not GCP ops.

| Daily question | Look at | Runtime | Fit |
| --- | --- | --- | --- |
| What did this project cost? | Cloud Billing API. No first-party billing MCP on the supported-products list. Community: [krzko/google-cloud-mcp](https://github.com/krzko/google-cloud-mcp), [RadiumGu/gcp-billing-and-monitoring-mcp](https://github.com/RadiumGu/gcp-billing-and-monitoring-mcp) | Node | **First cut.** Go SDK wrapper. |
| Alarms / logs | Cloud Monitoring (`https://monitoring.googleapis.com/mcp`); Logging (`logging.googleapis.com/mcp`); Error Reporting; Personalized Service Health | remote | Second. Local twin: `@google-cloud/observability-mcp`. |
| What does this service do? | Developer Knowledge API (`developerknowledge.googleapis.com/mcp`); Gemini Cloud Assist | remote | Search already answers a lot of this. |
| Just call the API | Cloud CLI Execution (`cloudcli.googleapis.com/mcp`); local `gcloud-mcp` | remote / Node+`gcloud` | Small tool count, huge blast radius. IAM is the ACL. Needs the `gcloud` CLI on the box — not distroless. |
| Places / geocode | Maps Grounding Lite / Maps Code Assist remote MCPs | remote | Skip — we already have `maps__*`. |

**Not doing from that catalog** (same reject as k8s / Grafana /
Paperless / the AWS dump): GKE, Cloud Run-as-an-IDE, Compute CRUD,
Cloud Storage as a vault, BigQuery / Bigtable / Spanner / AlloyDB /
Cloud SQL / Firestore, Pub/Sub, Dataproc, Vertex / Gemini Enterprise
Agent Platform, Firebase, Security Operations, IAM Policy
Troubleshooter as a toolbox.

First ship: a tiny Go `gcp-billing` (or `gcp__billing_*` on a small
`gcp` server) — list this month / last week / by SKU. Monitoring
alarms are a second prefix. Do not bake `npx @google-cloud/gcloud-mcp`.

When something ships: MCP page + `mcp.toml` snippet, then delete the
row here.
