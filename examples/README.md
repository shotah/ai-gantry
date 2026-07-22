# examples/

Templates for a first gantry deploy. These files are the **source of truth**.
`deploy/` is only a thin local-dev mount — regenerate it with:

```bash
make init
# or: go run ./cmd/gantry init
# env overrides: PERSONA_DIR=… MCP_MANIFEST=…
```

`gantry init` embeds these templates in the binary (skip existing files; fail-fast
if the target dirs are not writable).

| Path | Mounts / copies as |
| --- | --- |
| `persona/*.example.md` | `PERSONA_DIR` (default `/persona`) as `*.md` |
| `mcp.toml.example` | `MCP_MANIFEST` (default `/etc/gantry/mcp.toml`) |
| `env.example` | repo-root `.env.example` (then copy → `.env`) |

Persona concat order is fixed in `internal/persona.PreferredOrder`:

`SOUL` → `IDENTITY` → `USER` → `AGENTS` → `TOOLS` → `HEARTBEAT` → `BOOTSTRAP` → `MEMORY`,
then any other `*.md` alphabetically. Missing files are skipped.

**Publishing filters** in `mcp.toml` (optional per server):

```toml
[[server]]
name    = "garmin"
command = "garmin"
args    = ["mcp"]
tools   = ["get_sleep", "get_weight", "get_hrv"]  # allowlist
# exclude = ["raw_*"]                               # denylist (* ?)
# tools_prefix = "garm"                             # default: name
```

**Do not commit** filled-in personal `*.md` (identity, email, house map). Commit
only `*.example.md` templates.

Slash commands once running: `/new`, `/status`, `/tools` (and stdio `/quit`).
Reload persona without restart: `kill -HUP <pid>` (unix).
