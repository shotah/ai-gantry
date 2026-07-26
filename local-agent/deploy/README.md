# Native host deploy (binary + systemd + Ollama)

Same agent as Docker compose — different supervisor. Layout on the box:

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

# optional: pull MCP tools out of an existing Docker TIM once
NATIVE_DOCKER_HOST=old-docker-host.example.com
NATIVE_DOCKER_CONTAINER=gantry

make remote-native-env      # write deploy/gantry.env from .env (Ollama defaults)
make remote-native-check
make remote-native-deploy   # fetch binary (+ tools) → scp stage → sudo install → start
make remote-native-logs
```

`remote-native-deploy` does **not** overwrite `data/gantry.db`. Migrate memory once (scp), then only ship binary/env/tools/persona.

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
| `remote-native-deploy` | fetch → sync → install → start |

## Cutover from Docker TIM

1. Stop old container (one Telegram token → one process).
2. Re-scp a fresh `gantry.db` if the old box kept writing.
3. `make remote-native-deploy` against the Beelink.
