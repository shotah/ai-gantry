#!/usr/bin/env bash
# Remote deploy helper (Unix / WSL / Ubuntu workstation → Ubuntu server)
# Usage: ./scripts/remote.sh <check|sync|sync-secret|up|down|restart|logs|ps|status|ssh> [remote cmd...]

set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

load_env() {
  [[ -f .env ]] || { echo "Missing .env"; exit 1; }
  set -a
  # shellcheck disable=SC1091
  source <(grep -E '^[A-Za-z_][A-Za-z0-9_]*=' .env | sed 's/\r$//')
  set +a
}

load_env
: "${DEPLOY_HOST:?Set DEPLOY_HOST in .env}"
DEPLOY_USER="${DEPLOY_USER:-ubuntu}"
DEPLOY_PATH="${DEPLOY_PATH:-/opt/gantry}"
DEPLOY_SSH_PORT="${DEPLOY_SSH_PORT:-22}"

SSH_OPTS=(-p "$DEPLOY_SSH_PORT" -o StrictHostKeyChecking=accept-new)
# scp: -P is port; -p is preserve (do not reuse SSH_OPTS for scp)
SCP_OPTS=(-P "$DEPLOY_SSH_PORT" -o StrictHostKeyChecking=accept-new)
[[ -n "${DEPLOY_SSH_KEY:-}" ]] && SSH_OPTS+=(-i "$DEPLOY_SSH_KEY") && SCP_OPTS+=(-i "$DEPLOY_SSH_KEY")
TARGET="${DEPLOY_USER}@${DEPLOY_HOST}"

remote() {
  ssh "${SSH_OPTS[@]}" "$TARGET" "$@"
}

ACTION="${1:-}"
shift || true

read_manifest_lines() {
  local manifest="$1"
  [[ -f "$manifest" ]] || { echo "Missing manifest: $manifest"; exit 1; }
  grep -vE '^[[:space:]]*(#|$)' "$manifest" | sed -e 's/\r$//' -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//'
}

ensure_remote_parents() {
  local dirs=(data)
  local f
  for f in "$@"; do
    [[ "$f" == */* ]] && dirs+=("${f%/*}")
  done
  local mkdir_args="" d
  while IFS= read -r d; do
    [[ -n "$d" ]] || continue
    mkdir_args+=" '$DEPLOY_PATH/$d'"
  done < <(printf '%s\n' "${dirs[@]}" | sort -u)
  echo "Ensuring remote dirs under $DEPLOY_PATH"
  # shellcheck disable=SC2086
  remote "mkdir -p$mkdir_args"
}

copy_to_remote() {
  local rel="$1"
  if [[ -d "$rel" ]]; then
    remote "mkdir -p '$DEPLOY_PATH/$rel'"
    echo "scp -r $rel/"
    scp "${SCP_OPTS[@]}" -r "$rel" "$TARGET:$DEPLOY_PATH/${rel%/*}/"
  elif [[ -f "$rel" ]]; then
    echo "scp $rel"
    scp "${SCP_OPTS[@]}" "$rel" "$TARGET:$DEPLOY_PATH/$rel"
  else
    echo "Skip missing $rel"
    return 1
  fi
}

case "$ACTION" in
  check)
    echo "Checking SSH: $TARGET:$DEPLOY_SSH_PORT → $DEPLOY_PATH"
    remote "echo ok && uname -a && docker --version && docker compose version"
    ;;
  sync)
    # Code/config only — credentials use sync-secret (see secrets-manifest.txt)
    mapfile -t files < <(read_manifest_lines scripts/deploy-manifest.txt)
    ensure_remote_parents "${files[@]}"
    for f in "${files[@]}"; do
      copy_to_remote "$f" || true
    done
    # Persona is now SOUL/RULES/USER/TOOLS — scp does not delete leftovers
    echo "Cleaning obsolete persona files on remote (if present)"
    remote "cd '$DEPLOY_PATH/persona' 2>/dev/null && rm -f IDENTITY.md AGENTS.md MEMORY.md HEARTBEAT.md BOOTSTRAP.md || true"
    echo "Synced to $TARGET:$DEPLOY_PATH"
    echo "Note: data/.config secrets are NOT in remote-deploy. Use make garmin-sync / strava-sync / ytmusic-sync / google-sync"
    ;;
  sync-secret)
    name="$(echo "${1:-}" | tr '[:upper:]' '[:lower:]')"
    shift || true
    [[ -n "$name" ]] || { echo "Usage: $0 sync-secret <garmin|strava|ytmusic|google|all>"; exit 1; }

    paths=()
    while IFS= read -r line; do
      group="${line%%|*}"
      path="${line#*|}"
      group="$(echo "$group" | tr '[:upper:]' '[:lower:]' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
      path="$(echo "$path" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
      if [[ "$name" == "all" || "$group" == "$name" ]]; then
        # Only local paths that exist (skip optional legacy secrets/google/*).
        [[ -e "$path" ]] && paths+=("$path")
      fi
    done < <(read_manifest_lines scripts/secrets-manifest.txt)

    if [[ ${#paths[@]} -eq 0 ]]; then
      echo "No local files for '$name' — run the matching *-auth first (see scripts/secrets-manifest.txt)"
      exit 1
    fi

    # Stage under /tmp, then sudo install (native /opt/gantry is owned by gantry).
    STAGE=/tmp/gantry-secret-stage
    echo "Staging secrets under $TARGET:$STAGE then sudo install -> $DEPLOY_PATH"
    echo "(sudo may prompt for your password)"
    remote "rm -rf '$STAGE' && mkdir -p '$STAGE'"
    for p in "${paths[@]}"; do
      if [[ -d "$p" ]]; then
        remote "mkdir -p '$STAGE/${p%/*}'"
        echo "scp -r $p/ -> stage"
        scp "${SCP_OPTS[@]}" -r "$p" "$TARGET:$STAGE/${p%/*}/"
      else
        remote "mkdir -p '$STAGE/$(dirname "$p")'"
        echo "scp $p -> stage"
        scp "${SCP_OPTS[@]}" "$p" "$TARGET:$STAGE/$p"
      fi
    done
    {
      echo '#!/bin/bash'
      echo 'set -euo pipefail'
      echo "STAGE='$STAGE'"
      echo "DEST='$DEPLOY_PATH'"
      echo 'OWNER="$(stat -c %U "$DEST/data" 2>/dev/null || true)"'
      echo 'if [ -z "${OWNER:-}" ]; then'
      echo '  if id gantry >/dev/null 2>&1; then OWNER=gantry; else OWNER="$(logname 2>/dev/null || echo root)"; fi'
      echo 'fi'
      echo "for rel in ${paths[*]}; do"
      echo '  mkdir -p "$DEST/$(dirname "$rel")"'
      echo '  if [ -d "$STAGE/$rel" ]; then'
      echo '    mkdir -p "$DEST/$rel"'
      echo '    cp -a "$STAGE/$rel/." "$DEST/$rel/"'
      echo '  else'
      echo '    cp -a "$STAGE/$rel" "$DEST/$rel"'
      echo '  fi'
      echo '  chown -R "$OWNER:$OWNER" "$DEST/$rel"'
      echo '  echo "installed $rel (owner=$OWNER)"'
      echo 'done'
      echo 'rm -rf "$STAGE"'
    } > /tmp/gantry-install-secrets.sh
    scp "${SCP_OPTS[@]}" /tmp/gantry-install-secrets.sh "$TARGET:/tmp/gantry-install-secrets.sh"
    ssh "${SSH_OPTS[@]}" -t "$TARGET" 'sudo bash /tmp/gantry-install-secrets.sh'
    echo "Secret group '$name' synced to $TARGET:$DEPLOY_PATH (${#paths[@]} path(s))"
    ;;
  up)
    bust=$(date +%s)
    remote "cd '$DEPLOY_PATH' && docker compose build --pull --build-arg TOOLS_CACHEBUST=$bust && docker compose up -d"
    ;;
  down) remote "cd '$DEPLOY_PATH' && docker compose down" ;;
  restart) remote "cd '$DEPLOY_PATH' && docker compose restart" ;;
  logs) ssh "${SSH_OPTS[@]}" -t "$TARGET" "cd '$DEPLOY_PATH' && docker compose logs -f --tail=100" ;;
  ps) remote "cd '$DEPLOY_PATH' && docker compose ps" ;;
  status) remote "cd '$DEPLOY_PATH' && docker compose exec -T gantry /usr/local/bin/gantry status && echo OK" ;;
  ssh)
    if [[ $# -gt 0 ]]; then
      ssh "${SSH_OPTS[@]}" -t "$TARGET" "cd '$DEPLOY_PATH' && $*"
    else
      ssh "${SSH_OPTS[@]}" -t "$TARGET" "cd '$DEPLOY_PATH' && exec \$SHELL -l"
    fi
    ;;
  *)
    echo "Usage: $0 <check|sync|sync-secret|up|down|restart|logs|ps|status|ssh>"
    exit 1
    ;;
esac
