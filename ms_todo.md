# Microsoft stack MCP — hunt

Attack this file. Kernel loop work stays in [todo.md](todo.md).
Siblings: [apple_todo.md](apple_todo.md) · [aws_todo.md](aws_todo.md) ·
[gcp_todo.md](gcp_todo.md).

Prefix enable is why a fat Graph catalog can sit **off** until
`microsoft__todo`. That does **not** lift fit gate 3 — `npx` / `uvx` /
Python stay rejected. The list is “what daily question, what API,
write-or-import a static binary (Go / C / C++ / Rust).” Remote hosted
MCP also waits on **Outbound HTTP MCP** in `todo.md`.

---

**Need: WORK.** Outlook / Teams / To Do. Separate from `google-mcp`.
Prefix enable per family (`microsoft__mail`, …).

Graph is **one** API; every community server is a fat catalog
(40–300 tools). Prefix enable is the only reason to look.

## Tasks

| App | Who | Graph | Use |
| --- | --- | --- | --- |
| **Microsoft To Do** | Person | `/me/todo` (`Tasks.ReadWrite`) | Google Tasks analogue. Outlook Tasks moved here. Assigned Planner items often show up in To Do. **Start here.** |
| **Planner** | Team / Group | Planner APIs | Boards in Teams. Not a personal list. |
| Flagged mail | Inbox | Mail | Not a task app. |
| Loop / Project | Some orgs | | Not the first cut. |

## Official Microsoft MCP (mostly the wrong shape)

| Package | What it actually is |
| --- | --- |
| Agent 365 Work IQ Mail / Calendar (`mcp_MailTools`, `mcp_CalendarTools`) | First-party Graph mail+calendar. Copilot-licensed org, Entra admin, remote `agent365.svc.cloud.microsoft`. No personal Outlook.com. Needs outbound HTTP MCP. |
| [microsoft/EnterpriseMCP](https://github.com/microsoft/enterprisemcp) | Entra directory query only. Not Outlook. |
| [Azure MCP](https://github.com/microsoft/mcp/tree/main/servers/Azure.Mcp.Server) (`@azure/mcp`, .NET) | Azure *cloud* (40+ services). Not mail/Teams. Same “ops catalog” smell as AWS EKS. |

## Community Graph wrappers (Node/TS — do not npx; steal the surface)

| Package | Surface | Size |
| --- | --- | --- |
| [softeria/ms-365-mcp-server](https://github.com/softeria/ms-365-mcp-server) | Mail, calendar, files, Excel, OneNote, To Do, Planner, Teams (org-mode), 300+ 1:1 Graph tools, `--preset mail\|calendar\|tasks\|teams` | Kitchen sink with presets — copy the preset idea, not the 300. |
| [sam2kb/m365-mcp](https://github.com/sam2kb/m365-mcp) | Mail, calendar, contacts, OneDrive, Teams, To Do (~43) | Delegated device-code, no client secret. |
| [@sapientsai microsoft365-mcp-server](https://www.npmjs.com/package/microsoft365-mcp-server) | 73 tools / 12 domains including Teams, Planner, To Do | FastMCP / Node. |
| [ButylCompound/tasks-mcp](https://github.com/ButylCompound/tasks-mcp) | To Do + Planner + Outlook calendar only | Closest to a tiny tasks cut. |
| [stefanskiasan/outlook-mcp](https://github.com/stefanskiasan/outlook-mcp) | Outlook + Teams, 75+ | Fat. |

First cut: a Go `microsoft-mcp` shaped like `google-mcp` — **mail +
calendar + todo**, `--preset everyday`, optional `teams` / `planner`
/ `onedrive`. Names `microsoft__mail_*`, `microsoft__calendar_*`,
`microsoft__todo_*`, `microsoft__teams_*`. Entra app + delegated
OAuth (same `/auth` story as Google). Personal Outlook.com vs work
tenant is a Graph permission matrix, not a second binary.

Teams is org-only and a send-surface — read / search first, send later.

When something ships: MCP page + `mcp.toml` snippet, then delete the
row here.
