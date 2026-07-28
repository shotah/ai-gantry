# Native host deploy (binary + systemd + Ollama)

Same agent as Docker compose — different supervisor. Product overview and
why native + Qwen: **[../../docs/deploy-native.md](../../docs/deploy-native.md)**.

Layout on the box:

```text
/opt/gantry/
  gantry              # shotah/ai-gantry release binary
  gantry.env          # EnvironmentFile (secrets)
  gantry.service → /etc/systemd/system/gantry.service
  mcp.toml
  bin/                # MCP tool binaries (PATH)
  persona/
  data/               # gantry.db + .config secrets
```

## One-time host prep

Done once on a clean Ubuntu box (see repo `todo.md` → Local Ubuntu + local model):

- Ollama + model (`qwen3.5:35b`)
- system user `gantry` + `/opt/gantry` tree

## From this workstation

```bash
# in local-agent/.env
DEPLOY_HOST=myserver.example.com   # or LAN IP
DEPLOY_USER=ubuntu
DEPLOY_PATH=/opt/gantry
DEPLOY_SSH_KEY=...

# optional: one-time MCP tools from a *separate* Docker TIM (not DEPLOY_HOST)
# NATIVE_DOCKER_HOST=old-docker-host.example.com

make remote-native-env      # write deploy/gantry.env from .env (Ollama defaults)
make remote-native-check
make remote-native-deploy   # fetch GitHub release → scp stage → sudo install → start
make remote-native-logs

# Dev loop (no release): cross-build this checkout → scp → install → restart
make remote-native-deploy-dev
```

`remote-native-deploy` / `remote-native-deploy-dev` do **not** overwrite `data/gantry.db`. Migrate memory once (scp), then only ship binary/env/tools/persona.

MCP credentials live under **`data/.config/`** (same relative paths as Docker):

```text
data/.config/google-mcp/credentials/
data/.config/strava/tokens.json
data/.config/garmin/session.json
data/.config/ytmusic/headers.json
```

Push from the workstation with `make google-sync` / `secrets-sync` (uses
`DEPLOY_PATH`, e.g. `/opt/gantry`). One-time from legacy `secrets/*`:
`make secrets-migrate`.

Auth on the host (no Make required) — flows declared in `mcp.toml`:

```bash
# load env so OAuth client ids / paths are set
set -a && source /opt/gantry/gantry.env && set +a
gantry auth                 # list
gantry auth strava
gantry auth garmin
gantry auth ytmusic
# Google Workspace MCP has no auth subcommand yet → make google-auth on a workstation
```

## Make targets

| Target | What it does |
| --- | --- |
| `remote-native-env` | Build `deploy/gantry.env` from `.env` + Ollama LLM_* |
| `remote-native-check` | SSH + ollama + `/opt/gantry` |
| `remote-native-fetch` | Download gantry release; optional docker-cp tools |
| `remote-native-sync` | Stage files under `/tmp/gantry-native` on the host |
| `remote-native-install` | `sudo install.sh` (may prompt for password) |
| `remote-native-up` / `down` / `restart` | systemctl |
| `remote-native-logs` / `status` | journalctl / `gantry status` |
| `remote-native-deploy` | fetch release → sync → install → start |
| `remote-native-deploy-dev` | cross-build working tree → sync → install → start |
| `remote-native-build-dev` | linux/amd64 → `.cache/native/gantry` only |

## Cutover from Docker TIM

1. Stop old container (one Telegram token → one process).
2. Re-scp a fresh `gantry.db` if the old box kept writing.
3. `make remote-native-deploy` against the Beelink.
