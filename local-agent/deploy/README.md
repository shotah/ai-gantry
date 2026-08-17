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

Done once on a clean Ubuntu box (see [docs/deploy-native.md](../../docs/deploy-native.md)):

- Ollama + model (`qwen3.6:35b-a3b`)
- system user `gantry` + `/opt/gantry` tree

## From this workstation

```bash
# in local-agent/.env
DEPLOY_HOST=myserver.example.com   # or LAN IP
DEPLOY_USER=ubuntu
DEPLOY_PATH=/opt/gantry
DEPLOY_SSH_KEY=...

make remote-native-env      # write deploy/gantry.env from .env (Ollama defaults)
make remote-native-check
make remote-native-deploy   # gantry release + tools-fetch → scp stage → sudo install → start
make remote-native-logs

# Dev loop (no release): cross-build this checkout → tools-fetch → scp → install → restart
make remote-native-deploy-dev

# Quick iterate (gantry + persona + env only — leave /opt/gantry/bin alone; no GitHub)
make remote-native-deploy-dev-quick
# or: NATIVE_SKIP_TOOLS=1 make remote-native-deploy-dev
```

MCP binaries come from `mcp.toml` `download_*` via `gantry tools-fetch` (skip when
the resolved archive is already cached). `download_tag=latest` calls the GitHub
API and can 403 when unauthenticated rate limits trip. Use **deploy-dev-quick**
for kernel/persona loops; run a full `deploy-dev` / `remote-native-fetch` when
you need new MCP binaries. Optional: set `GITHUB_TOKEN` to raise the API limit.

Model pin: set `NATIVE_LLM_MODEL=qwen3.6:35b-a3b` in `.env`, then
`make remote-native-env` (rewrites `deploy/gantry.env`). Deploy / deploy-dev
**reuse** an existing `gantry.env` — they do not regenerate it, so a stale
`NATIVE_LLM_MODEL` cannot silently overwrite a local edit on every ship.

### Spark of life (opt-in)

Random presence pings (short authentic check-ins). **Off unless `SPARK_QTY` is set** in
`.env`. `make remote-native-env` copies `SPARK_*` into `deploy/gantry.env` when
qty is set; otherwise those keys are omitted.

```bash
# in local-agent/.env
SPARK_QTY=4-6
SPARK_START_HOUR=6
SPARK_END_HOUR=21
# SPARK_SKIP_RECENT_MINUTES=60
# SPARK_PROMPT=...   # optional; one variant per line (\n); empty = built-in pool
```

Then `make remote-native-env` (or edit `deploy/gantry.env` by hand) and redeploy.
Full behavior: [../../docs/cron.md](../../docs/cron.md#spark-of-life-opt-in).

`remote-native-deploy` / `remote-native-deploy-dev` do **not** overwrite `data/gantry.db`. Migrate memory once (scp), then only ship binary/env/tools/persona.

MCP tools: sync stages binaries named in `mcp.toml` (from local cache). `install.sh`
removes anything in `/opt/gantry/bin` that is not in the stage — unless the stage
ships **no** tools (`-SkipTools` / `deploy-dev-quick`), in which case host bins are
left alone. Local stale copies under `.cache/native/bin` are pruned on fetch/sync.

Ollama tuning (keep-alive, `num_ctx`) ships as
[`ollama-gantry.conf`](ollama-gantry.conf) → `/etc/systemd/system/ollama.service.d/gantry.conf`.
`install.sh` reinstalls it **only when the file changes**, and then restarts
Ollama — so a tuning edit costs one cold turn, while ordinary redeploys leave
the model resident. Latency levers and how to read the perf logs:
[../../docs/deploy-native.md](../../docs/deploy-native.md#latency-measure-before-tuning).

MCP credentials live under **`data/.config/`** (same relative paths as Docker):

```text
data/.config/google-mcp/credentials/
data/.config/strava/tokens.json
data/.config/garmin/session.json
data/.config/youtube/oauth.json
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
gantry auth youtube
# Google Workspace MCP has no auth subcommand yet → make google-auth on a workstation
```

## Make targets

| Target | What it does |
| --- | --- |
| `remote-native-env` | **Rewrite** `deploy/gantry.env` from `.env` (`NATIVE_LLM_MODEL` / default `qwen3.6:35b-a3b`) |
| `remote-native-check` | SSH + ollama + `/opt/gantry` |
| `remote-native-fetch` | Download gantry release; optional docker-cp tools |
| `remote-native-sync` | Stage files under `/tmp/gantry-native` on the host |
| `remote-native-install` | `sudo install.sh` (may prompt for password) |
| `remote-native-up` / `down` / `restart` | systemctl |
| `remote-native-logs` / `status` | journalctl / `gantry status` |
| `remote-native-deploy` | fetch release → sync → install → start |
| `remote-native-deploy-dev` | cross-build working tree → sync → install → start |
| `remote-native-build-dev` | linux/amd64 → `.cache/native/gantry` only |

## Cutover from Docker SAM

1. Stop old container (one Telegram token → one process).
2. Re-scp a fresh `gantry.db` if the old box kept writing.
3. `make remote-native-deploy` against the Beelink.
