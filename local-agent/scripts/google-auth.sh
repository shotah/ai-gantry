#!/usr/bin/env bash
# One-shot Google Workspace OAuth via a throwaway Python container.
# Writes secrets/google-mcp/credentials/<email>.json (MCP format).
# No local gws / Python required — only Docker + .env.
# Usage (repo root):  make google-auth
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${GOOGLE_AUTH_PORT:-4100}"
DOTENV="$ROOT/.env"
CRED_DIR="$ROOT/secrets/google-mcp/credentials"
SCRIPT="$ROOT/scripts/google-auth-inner.py"

dotenv_get() {
  local key="$1" line val
  [[ -f "$DOTENV" ]] || return 0
  line="$(grep -E "^[[:space:]]*${key}=" "$DOTENV" | tail -n1 || true)"
  [[ -n "$line" ]] || return 0
  val="${line#*=}"
  val="${val#"${val%%[![:space:]]*}"}"
  val="${val%"${val##*[![:space:]]}"}"
  if [[ "$val" == \"*\" && "$val" == *\" ]]; then
    val="${val:1:${#val}-2}"
  elif [[ "$val" == \'*\' && "$val" == *\' ]]; then
    val="${val:1:${#val}-2}"
  fi
  printf '%s' "$val"
}

EMAIL="$(dotenv_get USER_GOOGLE_EMAIL)"
CLIENT_ID="$(dotenv_get GOOGLE_OAUTH_CLIENT_ID)"
CLIENT_SECRET="$(dotenv_get GOOGLE_OAUTH_CLIENT_SECRET)"

[[ -n "$EMAIL" ]] || { echo "Set USER_GOOGLE_EMAIL in .env" >&2; exit 1; }
[[ -n "$CLIENT_ID" && -n "$CLIENT_SECRET" ]] || {
  echo "Set GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET in .env" >&2
  exit 1
}
[[ -f "$SCRIPT" ]] || { echo "Missing $SCRIPT" >&2; exit 1; }

mkdir -p "$CRED_DIR"
STALE="$CRED_DIR/${EMAIL}.json"
if [[ -f "$STALE" ]]; then
  rm -f "$STALE"
  echo "Cleared stale credential: $STALE"
fi

echo "Google OAuth via container (python:3.12-slim). No local gws."
echo "Ensure the Desktop OAuth client allows http://localhost:${PORT}/oauth2callback"
echo

docker run --rm -it \
  -p "${PORT}:${PORT}" \
  -e "GOOGLE_OAUTH_CLIENT_ID=${CLIENT_ID}" \
  -e "GOOGLE_OAUTH_CLIENT_SECRET=${CLIENT_SECRET}" \
  -e "USER_GOOGLE_EMAIL=${EMAIL}" \
  -e "GOOGLE_AUTH_PORT=${PORT}" \
  -e "WORKSPACE_MCP_CREDENTIALS_DIR=/data/credentials" \
  -v "${CRED_DIR}:/data/credentials" \
  -v "${SCRIPT}:/auth.py:ro" \
  python:3.12-slim-bookworm \
  python /auth.py

echo
echo "Next: make remote-deploy   (or make up)"
