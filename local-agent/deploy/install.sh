#!/bin/bash
# Install staged native gantry files into /opt/gantry and enable systemd.
# Stage dir is prepared by scripts/remote-native.ps1 (or .sh).
# Usage: sudo ./install.sh
set -euo pipefail

STAGE=/tmp/gantry-native
DEST=/opt/gantry

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

if [ ! -x "$STAGE/gantry" ]; then
  echo "Missing $STAGE/gantry — run make remote-native-sync first" >&2
  exit 1
fi

mkdir -p "$DEST/bin" "$DEST/data" "$DEST/persona"

install -m 0755 "$STAGE/gantry" "$DEST/gantry"

# Only touch DEST/bin when the stage actually shipped tools. Empty/missing stage
# bin means "leave host tools alone" (SkipTools / quick gantry-only deploys).
if [ -d "$STAGE/bin" ] && compgen -G "$STAGE/bin/*" > /dev/null; then
  find "$STAGE/bin" -maxdepth 1 -type f -executable -exec install -m 0755 {} "$DEST/bin/" \;
  # Drop binaries no longer staged (renamed/removed MCP tools, e.g. google-workspace-mcp-go → google-mcp).
  shopt -s nullglob
  for f in "$DEST/bin"/*; do
    base=$(basename "$f")
    if [ ! -e "$STAGE/bin/$base" ]; then
      echo "Removing obsolete tool: $DEST/bin/$base"
      rm -f "$f"
    fi
  done
  shopt -u nullglob
else
  echo "No tools in stage — leaving $DEST/bin unchanged"
fi

if [ -f "$STAGE/gantry.env" ]; then
  install -m 0640 -o gantry -g gantry "$STAGE/gantry.env" "$DEST/gantry.env"
fi

if [ -f "$STAGE/mcp.toml" ]; then
  install -m 0644 -o gantry -g gantry "$STAGE/mcp.toml" "$DEST/mcp.toml"
fi

# Only sync persona when the stage actually has markdown (never wipe with an empty dir).
if [ -d "$STAGE/persona" ] && compgen -G "$STAGE/persona/*.md" > /dev/null; then
  rsync -a --delete "$STAGE/persona/" "$DEST/persona/"
fi

# Never overwrite live SQLite from the stage (memory lives on the host).
# data/.config secrets are migrated separately.

install -m 0644 "$STAGE/gantry.service" /etc/systemd/system/gantry.service
chown -R gantry:gantry "$DEST"
chmod 0755 "$DEST/gantry"
chmod 0755 "$DEST/bin"/* 2>/dev/null || true

# Ollama tuning drop-in (keep-alive + context length) from ollama-gantry.conf.
# Reinstalled whenever the staged file changes, so tuning edits actually ship;
# unchanged means no restart, so a redeploy keeps the model resident.
# Delete the override file and restart ollama to revert.
if systemctl cat ollama.service > /dev/null 2>&1; then
  OLLAMA_OVERRIDE=/etc/systemd/system/ollama.service.d/gantry.conf
  if [ -f "$STAGE/ollama-gantry.conf" ]; then
    if ! cmp -s "$STAGE/ollama-gantry.conf" "$OLLAMA_OVERRIDE"; then
      mkdir -p /etc/systemd/system/ollama.service.d
      install -m 0644 "$STAGE/ollama-gantry.conf" "$OLLAMA_OVERRIDE"
      systemctl daemon-reload
      systemctl restart ollama
      echo "Updated ollama drop-in: $OLLAMA_OVERRIDE (ollama restarted; first turn is cold)"
    else
      echo "Ollama drop-in already current: $OLLAMA_OVERRIDE"
    fi
  elif [ ! -f "$OLLAMA_OVERRIDE" ]; then
    mkdir -p /etc/systemd/system/ollama.service.d
    printf '%s\n' '[Service]' 'Environment=OLLAMA_KEEP_ALIVE=-1' > "$OLLAMA_OVERRIDE"
    systemctl daemon-reload
    systemctl restart ollama
    echo "Installed ollama keep-alive override: $OLLAMA_OVERRIDE"
  fi
fi

systemctl daemon-reload
systemctl enable gantry.service

echo "Installed to $DEST"
echo "Start with: systemctl start gantry"
echo "Logs:       journalctl -u gantry -f"
