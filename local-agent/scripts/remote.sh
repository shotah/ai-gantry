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
    echo "Synced to $TARGET:$DEPLOY_PATH"
    echo "Note: token/session secrets are NOT in remote-deploy. Use make garmin-sync / strava-sync / ytmusic-sync / google-sync"
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
        paths+=("$path")
      fi
    done < <(read_manifest_lines scripts/secrets-manifest.txt)

    if [[ ${#paths[@]} -eq 0 ]]; then
      echo "Unknown secret group '$name'. See scripts/secrets-manifest.txt"
      exit 1
    fi

    ensure_remote_parents "${paths[@]}"
    copied=0
    for p in "${paths[@]}"; do
      if copy_to_remote "$p"; then
        copied=$((copied + 1))
      fi
    done
    [[ "$copied" -gt 0 ]] || { echo "No local files found for secret group '$name' — run the matching *-auth first"; exit 1; }

    if [[ "$name" == "google" || "$name" == "all" ]]; then
      remote "rm -f '$DEPLOY_PATH/secrets/google/token_cache.json'; rm -rf '$DEPLOY_PATH/secrets/google/cache'"
    fi
    echo "Secret group '$name' synced to $TARGET:$DEPLOY_PATH ($copied path(s))"
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
