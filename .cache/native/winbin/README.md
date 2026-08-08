# google-mcp

Google Workspace MCP server (Go)

<p align="center">
  <a href="https://github.com/shotah/google-mcp/actions/workflows/ci.yml"><img src="https://github.com/shotah/google-mcp/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/shotah/google-mcp/actions/workflows/release.yml"><img src="https://github.com/shotah/google-mcp/actions/workflows/release.yml/badge.svg" alt="Release"></a>
  <a href="https://github.com/shotah/google-mcp/actions/workflows/ci.yml"><img src="https://github.com/shotah/google-mcp/raw/gh-pages/badges/coverage.svg" alt="Coverage"></a>
  <a href="https://pkg.go.dev/github.com/shotah/google-mcp"><img src="https://pkg.go.dev/badge/github.com/shotah/google-mcp.svg" alt="Go Reference"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/shotah/google-mcp" alt="Go version">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/shotah/google-mcp" alt="License"></a>
</p>

<p align="center">
  <strong>Give Claude, Cursor, and other MCP clients real access to your Google Workspace.</strong><br>
  Gmail, Drive, Calendar, Docs, Sheets, and more — one small binary, no Python runtime.
</p>

**138 tools · 12 services · single binary · OAuth that just works**

Drop it into your MCP config and ask your agent to search mail, clean up calendar duplicates, draft Docs, or update Sheets — with a permission surface you control (`read` / `edit` / `complete`).

| Service | What agents can do |
| ------- | ------------------ |
| **Gmail** | Search, read, send, labels, filters |
| **Drive** | Search, read, create, share |
| **Calendar** | List, create, modify, delete events |
| **Docs / Sheets / Slides** | Read and edit Workspace files |
| **Tasks · Contacts · Chat · Forms · Apps Script · Search** | Day-to-day Workspace automation |

Built for **local, single-user** AI tool use. Need multi-user OAuth 2.1 or an HTTP server? Use the original [Python server](https://github.com/taylorwilsdon/google_workspace_mcp) — same tool surface, different deployment model.

## Why this one

- **Zero runtime** — download a binary (or `go install`) and run; no venv, no `uv`, no dependency churn
- **Service-first tool names** — `google__calendar_list_events`, not ambiguous `get_events` (agent clarity over Python string parity)
- **Agent-friendly filters** — `--preset everyday` (personal assistant) or `lean` (tiny models), or explicit `--tools` / `--tool-tier` / `--capability`
- **Local-first auth** — `google-mcp auth` once; tokens under `~/.google_workspace_mcp/credentials/`; MCP refreshes silently
- **Works where you already work** — Claude Code, Cursor, and any stdio MCP client

## Quick start

### 1. Google Cloud OAuth

Reuse credentials from the [Python server](https://github.com/taylorwilsdon/google_workspace_mcp) if you already have them. Otherwise:

1. Open [Google Cloud Console](https://console.cloud.google.com/)
2. Create or select a project → **APIs & Services → OAuth consent screen**
3. **Credentials → Create Credentials → OAuth Client ID → Desktop Application**
4. Copy the **Client ID** and **Client Secret**
5. Enable only the APIs you need:

<details>
<summary><strong>Enable APIs</strong> (click to expand)</summary>

- [Gmail API](https://console.cloud.google.com/flows/enableapi?apiid=gmail.googleapis.com)
- [Google Drive API](https://console.cloud.google.com/flows/enableapi?apiid=drive.googleapis.com)
- [Google Calendar API](https://console.cloud.google.com/flows/enableapi?apiid=calendar-json.googleapis.com)
- [Google Docs API](https://console.cloud.google.com/flows/enableapi?apiid=docs.googleapis.com)
- [Google Sheets API](https://console.cloud.google.com/flows/enableapi?apiid=sheets.googleapis.com)
- [Google Slides API](https://console.cloud.google.com/flows/enableapi?apiid=slides.googleapis.com)
- [Google Forms API](https://console.cloud.google.com/flows/enableapi?apiid=forms.googleapis.com)
- [Google Tasks API](https://console.cloud.google.com/flows/enableapi?apiid=tasks.googleapis.com)
- [Google Chat API](https://console.cloud.google.com/flows/enableapi?apiid=chat.googleapis.com)
- [People API (Contacts)](https://console.cloud.google.com/flows/enableapi?apiid=people.googleapis.com)
- [Apps Script API](https://console.cloud.google.com/flows/enableapi?apiid=script.googleapis.com)
- [Custom Search API](https://console.cloud.google.com/flows/enableapi?apiid=customsearch.googleapis.com) *(optional)*

</details>

### 2. Install

**Pre-built binary** (no Go required) — grab the archive for your platform from [Releases](https://github.com/shotah/google-mcp/releases):

| Platform | File |
| --- | --- |
| Linux x86_64 | `google-mcp_*_linux_amd64.tar.gz` |
| Linux ARM64 | `google-mcp_*_linux_arm64.tar.gz` |
| macOS Apple Silicon | `google-mcp_*_darwin_arm64.tar.gz` |
| macOS Intel | `google-mcp_*_darwin_amd64.tar.gz` |
| Windows x86_64 | `google-mcp_*_windows_amd64.zip` |

```bash
tar xzf google-mcp_*_linux_amd64.tar.gz
chmod +x google-mcp
mv google-mcp ~/.local/bin/
```

**Or with Go** (1.26+):

```bash
go install github.com/shotah/google-mcp@latest
```

### 3. Environment

```bash
export GOOGLE_OAUTH_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export GOOGLE_OAUTH_CLIENT_SECRET="your-client-secret"
export USER_GOOGLE_EMAIL="you@gmail.com"  # optional but recommended
```

### 4. MCP client config

Use server id **`google`** so hosts expose tools as `google__calendar_list_events` (short server + service-prefixed tool).

**Personal assistant (recommended default)** — mail, calendar, Docs, Sheets, Tasks, Contacts, Drive (~34 core tools). Resolve people, find files by name, draft notes, schedule, todos:

```json
{
  "mcpServers": {
    "google": {
      "command": "google-mcp",
      "args": ["--preset", "everyday"],
      "env": {
        "GOOGLE_OAUTH_CLIENT_ID": "your-client-id.apps.googleusercontent.com",
        "GOOGLE_OAUTH_CLIENT_SECRET": "your-client-secret",
        "USER_GOOGLE_EMAIL": "you@gmail.com"
      }
    }
  }
}
```

**Tiny local models** (e.g. Qwen 35B) — starve harder with `--preset lean` (~11 tools: Gmail + Calendar only).

If OAuth env vars are already exported in the shell that launches your MCP client, omit the `env` block:

```json
{
  "mcpServers": {
    "google": {
      "command": "google-mcp",
      "args": ["--preset", "everyday"]
    }
  }
}
```

#### Which services do I need?

| Goal | Enable | Notes |
| --- | --- | --- |
| Track stats / log rows in a Sheet | `sheets` (in `everyday`) | `sheets_create_spreadsheet`, then `sheets_modify_values` / `sheets_read_values` |
| Draft / edit a Doc | `docs` (in `everyday`) | `docs_create` / `docs_get_content` / `docs_modify_text` |
| Find a file / Doc by name | `drive` (in `everyday`) | `drive_search_files`, then Docs/Sheets tools with the id |
| Find a sheet by name (Sheets-only) | `sheets` + `--tool-tier extended` | `sheets_list_spreadsheets` (uses Drive API under the hood) |
| Invite / email someone by name | `contacts` (in `everyday`) | `contacts_search` → email → calendar/gmail |
| Tasks / todos | `tasks` (in `everyday`) | Use `task_list_id="@default"` for the account default list |

OAuth already requests Drive + Docs + Sheets + People scopes on `google-mcp auth`. Trim with `--tools` if a persona needs a smaller surface.

### 5. Authenticate once (human / CLI — not an agent tool)

Run OAuth yourself before starting the MCP server:

```bash
export GOOGLE_OAUTH_CLIENT_ID="....apps.googleusercontent.com"
export GOOGLE_OAUTH_CLIENT_SECRET="...."
export USER_GOOGLE_EMAIL="you@gmail.com"   # optional

google-mcp auth
# alias: google-mcp login
# optional: google-mcp auth --email you@gmail.com
```

A browser opens; after you approve, tokens land in `~/.google_workspace_mcp/credentials/{email}.json`. You can copy that file onto an agent host. The MCP server refreshes access tokens automatically on API calls — small models should not be asked to call `auth_start`.

> `auth_start` remains only as a rare re-auth escape hatch (gmail / complete tier). Lean surfaces omit it.

## Configuration

### CLI flags

| Flag / command | Description | Default |
| --- | --- | --- |
| `auth` / `login` | First-time OAuth (human CLI); writes credential JSON | — |
| `--preset` | Named surface (see below) | unset |
| `--tools` | Services to enable (e.g. `gmail calendar docs sheets`) | all |
| `--tool-tier` | Depth: `core`, `extended`, or `complete` | `complete` |
| `--capability` | Permissions: `read`, `edit`, or `complete` | `complete` |
| `--read-only` | Shorthand for `--capability read` | `false` |

`--preset` fills tools / tier / capability only when those flags are omitted. Explicit flags always win.

| Preset | Services | Tier / capability | ~Tools | Use when |
| --- | --- | --- | --- | --- |
| `everyday` | gmail, calendar, docs, sheets, tasks, contacts, drive | core / edit | ~34 | Personal assistant (recommended) |
| `lean` | gmail, calendar | core / edit | ~11 | Tiny local models; mail + calendar only |

### Tool tiers (how deep each service goes)

`--tool-tier` is **not** “which Google products” — that’s `--tools` / `--preset`. Tier controls **depth inside** each enabled service (see `tools/tiers.go`):

| Tier | Meaning | Count (all services) |
| --- | --- | --- |
| `core` | Everyday path an assistant actually needs | 49 |
| `extended` | Discovery / management (list-by-name, filters, …) | 94 |
| `complete` | Rare / power-user extras | 138 |

Example: `--tools sheets --tool-tier core` → create/read/write cells. Need “find my sheet by name”? Use `extended` (`sheets_list_spreadsheets`) — still no Drive tools.

### Capabilities (what agents may do)

| Capability | Description | Count |
| --- | --- | --- |
| `read` | Read-only (same as `--read-only`) | 60 |
| `edit` | Everyday create/modify/delete; blocks high-impact ops | 132 |
| `complete` | Full surface including ownership transfer & bulk deletes | 138 |

Withheld under `edit`: `drive_transfer_ownership`, `contacts_batch_delete`, `tasks_delete_tasklist`, `contacts_delete_group`, `appscript_delete_project`, `tasks_clear_completed`.

### Environment variables

| Variable | Required | Description |
| --- | --- | --- |
| `GOOGLE_OAUTH_CLIENT_ID` | Yes | OAuth 2.0 Client ID |
| `GOOGLE_OAUTH_CLIENT_SECRET` | Yes | OAuth 2.0 Client Secret |
| `USER_GOOGLE_EMAIL` | No | Default account email |
| `WORKSPACE_MCP_CREDENTIALS_DIR` | No | Override credential directory |
| `GOOGLE_PSE_API_KEY` | No | Programmable Search Engine key |
| `GOOGLE_PSE_ENGINE_ID` | No | Programmable Search Engine ID |

### Available services

`gmail` `drive` `calendar` `docs` `sheets` `slides` `forms` `tasks` `chat` `contacts` `search` `appscript`

### Tool naming

Every tool is `{service}_{verb}_{object}` (snake_case). The MCP server name is `google`, so hosts typically show:

| Intent | Tool |
| --- | --- |
| Calendar tomorrow | `google__calendar_list_events` |
| Add a meeting | `google__calendar_create_event` |
| Search mail | `google__gmail_search_messages` |
| Send mail | `google__gmail_send_message` |
| Task list | `google__tasks_list_tasks` |
| Chat post | `google__chat_send_message` |

## When to use Go vs Python

| | This repo (Go) | [Python original](https://github.com/taylorwilsdon/google_workspace_mcp) |
| --- | --- | --- |
| Best for | Local Claude Code / Cursor / stdio MCP | Hosted or multi-user deployments |
| Install | Single binary | Python 3.10+ + deps |
| Tools | 138 (service-first names) | 137 (legacy names) |
| Transport | stdio | stdio + streamable HTTP |
| Auth | OAuth 2.0 (desktop) | OAuth 2.0 + OAuth 2.1 |
| Multi-user | No | Yes (sessions, Valkey, etc.) |

Same credentials work in both. Tool **names** diverge on purpose for agent routing.

## Tools

<details>
<summary><strong>Full tool reference (138 tools across 12 services)</strong></summary>

### Gmail (15 tools)

| Tool                                | Tier     | Description                             |
| ----------------------------------- | -------- | --------------------------------------- |
| `gmail_search_messages`             | core     | Search messages with Gmail query syntax |
| `gmail_get_message`         | core     | Get full message content                |
| `gmail_get_messages_batch`  | core     | Batch retrieve up to 25 messages        |
| `gmail_send_message`                | core     | Send email with optional attachments    |
| `gmail_get_attachment`      | extended | Download attachment content             |
| `gmail_get_thread`          | extended | Get full conversation thread            |
| `gmail_modify_message_labels`       | core     | Add/remove labels (trash/archive/spam)  |
| `gmail_list_labels`                 | extended | List all labels                         |
| `gmail_manage_label`                | extended | Create, update, or delete labels        |
| `gmail_draft_message`               | extended | Create draft email                      |
| `gmail_list_filters`                | extended | List mail filters                       |
| `gmail_create_filter`               | extended | Create new mail filter                  |
| `gmail_delete_filter`               | extended | Delete mail filter                      |
| `gmail_get_threads_batch`   | complete | Batch retrieve up to 25 threads         |
| `gmail_batch_modify_message_labels` | complete | Batch label operations                  |

**System tool** (registered with gmail, but not Gmail-specific):

| Tool                | Tier     | Description                       |
| ------------------- | -------- | --------------------------------- |
| `auth_start` | complete | Rare re-auth escape hatch (prefer `google-mcp auth`) |

### Google Drive (16 tools)

| Tool                             | Tier     | Description                          |
| -------------------------------- | -------- | ------------------------------------ |
| `drive_search_files`             | core     | Search files with Drive query syntax |
| `drive_get_file_content`         | core     | Download file content                |
| `drive_get_file_download_url`    | core     | Get download URL                     |
| `drive_create_file`              | core     | Create new file                      |
| `drive_import_to_doc`           | core     | Import file as Google Doc            |
| `drive_share_file`               | core     | Share file with users                |
| `drive_get_shareable_link`       | core     | Generate shareable link              |
| `drive_list_items`               | extended | List files in folder                 |
| `drive_copy_file`                | extended | Duplicate file                       |
| `drive_update_file`              | extended | Update file metadata/content         |
| `drive_update_permission`        | extended | Modify sharing permissions           |
| `drive_remove_permission`        | extended | Revoke access                        |
| `drive_transfer_ownership`       | extended | Transfer file ownership              |
| `drive_batch_share_file`         | extended | Batch sharing                        |
| `drive_get_file_permissions`     | complete | List all permissions                 |
| `drive_check_file_public_access` | complete | Check public sharing status          |

### Google Calendar (7 tools)

| Tool             | Tier     | Description                |
| ---------------- | -------- | -------------------------- |
| `calendar_list_calendars` | core     | List user's calendars      |
| `calendar_list_events`     | core     | List events in a time range |
| `calendar_get_event`       | core     | Get one event by event_id  |
| `calendar_create_event`   | core     | Create calendar event      |
| `calendar_update_event`   | core     | Update event details       |
| `calendar_delete_event`   | core     | Delete event               |
| `calendar_query_freebusy` | extended | Check availability         |

### Google Docs (19 tools)

| Tool                         | Tier     | Description                 |
| ---------------------------- | -------- | --------------------------- |
| `docs_get_content`            | core     | Get document text content   |
| `docs_create`                 | core     | Create new document         |
| `docs_modify_text`            | core     | Edit document text          |
| `docs_export_to_pdf`          | extended | Export as PDF               |
| `docs_search`                | extended | Search documents            |
| `docs_find_and_replace`       | extended | Find and replace text       |
| `docs_list_in_folder`        | extended | List docs in Drive folder   |
| `docs_insert_elements`        | extended | Insert formatted elements   |
| `docs_update_paragraph_style`     | extended | Change paragraph formatting |
| `docs_insert_image`           | complete | Insert image                |
| `docs_update_headers_footers` | complete | Edit headers/footers        |
| `docs_batch_update`           | complete | Batch document operations   |
| `docs_inspect_structure`      | complete | Analyze document structure  |
| `docs_create_table_with_data`     | complete | Create and populate table   |
| `docs_debug_table_structure`      | complete | Inspect table layout        |
| `docs_read_comments`     | complete | Read all comments           |
| `docs_create_comment`    | complete | Add comment                 |
| `docs_reply_to_comment`  | complete | Reply to comment            |
| `docs_resolve_comment`   | complete | Resolve comment             |

### Google Sheets (14 tools)

| Tool                            | Tier     | Description                  |
| ------------------------------- | -------- | ---------------------------- |
| `sheets_create_spreadsheet`            | core     | Create new spreadsheet       |
| `sheets_read_values`             | core     | Read cell values             |
| `sheets_modify_values`           | core     | Write/update cells           |
| `sheets_list_spreadsheets`             | extended | List user's spreadsheets     |
| `sheets_get_spreadsheet_info`          | extended | Get spreadsheet metadata     |
| `sheets_create_sheet`                  | complete | Add worksheet tab            |
| `sheets_format_range`            | complete | Format cell ranges           |
| `sheets_add_conditional_formatting`    | complete | Add conditional format rules |
| `sheets_update_conditional_formatting` | complete | Modify format rules          |
| `sheets_delete_conditional_formatting` | complete | Remove format rules          |
| `sheets_read_comments`     | complete | Read all comments            |
| `sheets_create_comment`    | complete | Add comment                  |
| `sheets_reply_to_comment`  | complete | Reply to comment             |
| `sheets_resolve_comment`   | complete | Resolve comment              |

### Google Slides (9 tools)

| Tool                            | Tier     | Description              |
| ------------------------------- | -------- | ------------------------ |
| `slides_create_presentation`           | core     | Create presentation      |
| `slides_get_presentation`              | core     | Get presentation content |
| `slides_batch_update`     | extended | Batch slide operations   |
| `slides_get_page`                      | extended | Get individual slide     |
| `slides_get_page_thumbnail`            | extended | Get slide thumbnail      |
| `slides_read_comments`    | complete | Read all comments        |
| `slides_create_comment`   | complete | Add comment              |
| `slides_reply_to_comment` | complete | Reply to comment         |
| `slides_resolve_comment`  | complete | Resolve comment          |

### Google Forms (6 tools)

| Tool                   | Tier     | Description             |
| ---------------------- | -------- | ----------------------- |
| `forms_create`          | core     | Create new form         |
| `forms_get`             | core     | Get form details        |
| `forms_list_responses`  | extended | List all responses      |
| `forms_set_publish_settings` | complete | Configure publishing    |
| `forms_get_response`    | complete | Get individual response |
| `forms_batch_update`    | complete | Batch form updates      |

### Google Tasks (12 tools)

| Tool                    | Tier     | Description             |
| ----------------------- | -------- | ----------------------- |
| `tasks_get_task`              | core     | Get task details        |
| `tasks_list_tasks`            | core     | List tasks              |
| `tasks_create_task`           | core     | Create task             |
| `tasks_update_task`           | core     | Update task             |
| `tasks_delete_task`           | extended | Delete task             |
| `tasks_list_tasklists`       | complete | List task lists         |
| `tasks_get_tasklist`         | complete | Get task list details   |
| `tasks_create_tasklist`      | complete | Create task list        |
| `tasks_update_tasklist`      | complete | Update task list        |
| `tasks_delete_tasklist`      | complete | Delete task list        |
| `tasks_move_task`             | complete | Move task between lists |
| `tasks_clear_completed` | complete | Clear completed tasks   |

### Google Chat (4 tools)

| Tool              | Tier     | Description       |
| ----------------- | -------- | ----------------- |
| `chat_send_message`    | core     | Send chat message |
| `chat_list_messages`    | core     | Get messages      |
| `chat_search_messages` | core     | Search messages   |
| `chat_list_spaces`     | extended | List spaces/DMs   |

### Google Contacts (15 tools)

| Tool                           | Tier     | Description          |
| ------------------------------ | -------- | -------------------- |
| `contacts_search`              | core     | Search contacts      |
| `contacts_get`                  | core     | Get contact details  |
| `contacts_list`                | core     | List contacts        |
| `contacts_create`               | core     | Create contact       |
| `contacts_update`               | extended | Update contact       |
| `contacts_delete`               | extended | Delete contact       |
| `contacts_list_groups`          | extended | List contact groups  |
| `contacts_get_group`            | extended | Get group details    |
| `contacts_batch_create`        | complete | Batch create         |
| `contacts_batch_update`        | complete | Batch update         |
| `contacts_batch_delete`        | complete | Batch delete         |
| `contacts_create_group`         | complete | Create group         |
| `contacts_update_group`         | complete | Update group         |
| `contacts_delete_group`         | complete | Delete group         |
| `contacts_modify_group_members` | complete | Manage group members |

### Google Custom Search (3 tools)

| Tool                         | Tier     | Description                      |
| ---------------------------- | -------- | -------------------------------- |
| `search_query`              | core     | Programmable Search Engine query |
| `search_query_siterestrict` | extended | Site-restricted search           |
| `search_get_engine_info`     | complete | Get search engine config         |

### Google Apps Script (17 tools)

| Tool                    | Tier     | Description             |
| ----------------------- | -------- | ----------------------- |
| `appscript_list_projects`  | core     | List user's scripts     |
| `appscript_get_project`    | core     | Get script metadata     |
| `appscript_get_content`    | core     | Get script source code  |
| `appscript_create_project` | core     | Create new script       |
| `appscript_update_content` | core     | Update script code      |
| `appscript_run_function`   | core     | Execute script function |
| `appscript_generate_trigger_code` | core     | Generate trigger code   |
| `appscript_create_deployment`     | extended | Deploy script           |
| `appscript_list_deployments`      | extended | List deployments        |
| `appscript_update_deployment`     | extended | Update deployment       |
| `appscript_delete_deployment`     | extended | Remove deployment       |
| `appscript_delete_project` | extended | Delete script           |
| `appscript_list_versions`         | extended | List versions           |
| `appscript_create_version`        | extended | Create version          |
| `appscript_get_version`           | extended | Get version details     |
| `appscript_list_processes` | extended | List running processes  |
| `appscript_get_metrics`    | extended | Get script metrics      |

</details>

## Credential storage

Tokens live as JSON under `~/.google_workspace_mcp/credentials/` (`{email}.json`). Directory is `0700`, files are `0600`. Override with `WORKSPACE_MCP_CREDENTIALS_DIR`.

## Development

```bash
go test ./...
go test ./tools/ -run TestGmail
INTEGRATION_TEST_EMAIL="you@gmail.com" go test -tags integration ./tools/
```

- **Unit** — formatting, parsing, helpers
- **Protocol** — MCP `tools/call` validation and error paths
- **Mock API** — handlers against `httptest.Server` fixtures
- **Integration** — real Google APIs (`integration` build tag; needs `INTEGRATION_TEST_EMAIL`)

## Limitations

- **stdio only** — no HTTP server mode ([Python version](https://github.com/taylorwilsdon/google_workspace_mcp) has that)
- **Single-user** — no multi-user sessions or OAuth 2.1
- **Local MCP clients** — not aimed at hosted multi-tenant deployments

## Acknowledgments

Go rewrite of [google_workspace_mcp](https://github.com/taylorwilsdon/google_workspace_mcp) by [Taylor Wilsdon](https://github.com/taylorwilsdon). The Python project remains the full-featured reference for multi-user and HTTP deployments. MIT licensed.

## License

[MIT](LICENSE)
