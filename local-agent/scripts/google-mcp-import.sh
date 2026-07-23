#!/usr/bin/env bash
# Convert secrets/google/credentials.json (gws export) into the magks MCP
# credential file format under secrets/google-mcp/credentials/<email>.json.
# Usage (repo root):  make google-mcp-import
# Requires: USER_GOOGLE_EMAIL, and either GOOGLE_OAUTH_* in .env or client_secret.json
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOTENV="$ROOT/.env"
GWS_CRED="$ROOT/secrets/google/credentials.json"
CLIENT_SECRET="$ROOT/secrets/google/client_secret.json"
OUT_DIR="$ROOT/secrets/google-mcp/credentials"

dotenv_get() {
  local key="$1"
  local line val
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

dotenv_set() {
  local key="$1" value="$2"
  local tmp
  tmp="$(mktemp)"
  if [[ -f "$DOTENV" ]] && grep -qE "^[[:space:]]*${key}=" "$DOTENV"; then
    # Replace existing key (portable: avoid sed -i differences).
    awk -v k="$key" -v v="$value" '
      BEGIN { re = "^[[:space:]]*" k "=" }
      $0 ~ re { print k "=" v; found=1; next }
      { print }
      END { if (!found) print k "=" v }
    ' "$DOTENV" >"$tmp"
  else
    if [[ -f "$DOTENV" ]]; then
      cat "$DOTENV" >"$tmp"
      [[ -s "$tmp" && "$(tail -c1 "$tmp" | od -An -tx1 | tr -d ' \n')" == "0a" ]] || echo >>"$tmp"
    else
      : >"$tmp"
    fi
    echo "# Google Workspace MCP (magks) - docs/google-workspace.md" >>"$tmp"
    echo "${key}=${value}" >>"$tmp"
  fi
  mv "$tmp" "$DOTENV"
}

EMAIL="$(dotenv_get USER_GOOGLE_EMAIL)"
if [[ -z "$EMAIL" ]]; then
  echo "Set USER_GOOGLE_EMAIL in .env (e.g. you@gmail.com)" >&2
  exit 1
fi

if [[ ! -f "$GWS_CRED" ]]; then
  echo "Missing $GWS_CRED - export via gws auth export first (docs/google-workspace.md)" >&2
  exit 1
fi

CLIENT_ID="$(dotenv_get GOOGLE_OAUTH_CLIENT_ID)"
CLIENT_SECRET_VAL="$(dotenv_get GOOGLE_OAUTH_CLIENT_SECRET)"

# Prefer python3 for JSON (present on Ubuntu deploy hosts); fall back to node.
export GWS_CRED CLIENT_SECRET CLIENT_ID CLIENT_SECRET_VAL EMAIL OUT_DIR
if command -v python3 >/dev/null 2>&1; then
  eval "$(python3 - <<'PY'
import json, os, sys

gws = json.load(open(os.environ["GWS_CRED"], encoding="utf-8"))
client_id = os.environ.get("CLIENT_ID") or ""
client_secret = os.environ.get("CLIENT_SECRET_VAL") or ""
secret_path = os.environ["CLIENT_SECRET"]
if (not client_id or not client_secret) and os.path.isfile(secret_path):
    cs = json.load(open(secret_path, encoding="utf-8"))
    installed = cs.get("installed") or cs.get("web") or {}
    client_id = client_id or installed.get("client_id") or ""
    client_secret = client_secret or installed.get("client_secret") or ""
client_id = client_id or (gws.get("client_id") or "")
client_secret = client_secret or (gws.get("client_secret") or "")
refresh = gws.get("refresh_token") or ""
if not client_id or not client_secret:
    print("echo Need GOOGLE_OAUTH_CLIENT_ID / GOOGLE_OAUTH_CLIENT_SECRET in .env \\(or client_secret.json\\) >&2; exit 1")
    sys.exit(0)
if not refresh:
    print("echo credentials.json has no refresh_token >&2; exit 1")
    sys.exit(0)

email = os.environ["EMAIL"]
out_dir = os.environ["OUT_DIR"]
os.makedirs(out_dir, mode=0o700, exist_ok=True)
out_path = os.path.join(out_dir, f"{email}.json")
payload = {
    "token": "",
    "refresh_token": refresh,
    "token_uri": "https://oauth2.googleapis.com/token",
    "client_id": client_id,
    "client_secret": client_secret,
    "scopes": [
        "https://www.googleapis.com/auth/gmail.modify",
        "https://www.googleapis.com/auth/drive",
        "https://www.googleapis.com/auth/calendar",
        "https://www.googleapis.com/auth/documents",
        "https://www.googleapis.com/auth/spreadsheets",
        "https://www.googleapis.com/auth/tasks",
        "https://www.googleapis.com/auth/contacts",
    ],
    "expiry": "2000-01-01T00:00:00Z",
}
with open(out_path, "w", encoding="utf-8", newline="\n") as f:
    json.dump(payload, f, indent=2)
    f.write("\n")
os.chmod(out_path, 0o600)

def sh_quote(s: str) -> str:
    return "'" + s.replace("'", "'\"'\"'") + "'"

print(f"OUT_PATH={sh_quote(out_path)}")
print(f"CLIENT_ID={sh_quote(client_id)}")
print(f"CLIENT_SECRET_VAL={sh_quote(client_secret)}")
PY
)"
elif command -v node >/dev/null 2>&1; then
  eval "$(node - <<'JS'
const fs = require('fs');
const path = require('path');
const gws = JSON.parse(fs.readFileSync(process.env.GWS_CRED, 'utf8'));
let clientId = process.env.CLIENT_ID || '';
let clientSecret = process.env.CLIENT_SECRET_VAL || '';
const secretPath = process.env.CLIENT_SECRET;
if ((!clientId || !clientSecret) && fs.existsSync(secretPath)) {
  const cs = JSON.parse(fs.readFileSync(secretPath, 'utf8'));
  const installed = cs.installed || cs.web || {};
  clientId = clientId || installed.client_id || '';
  clientSecret = clientSecret || installed.client_secret || '';
}
clientId = clientId || gws.client_id || '';
clientSecret = clientSecret || gws.client_secret || '';
const refresh = gws.refresh_token || '';
function shQuote(s) {
  return "'" + String(s).replace(/'/g, "'\"'\"'") + "'";
}
if (!clientId || !clientSecret) {
  console.log("echo Need GOOGLE_OAUTH_CLIENT_ID / GOOGLE_OAUTH_CLIENT_SECRET in .env \\(or client_secret.json\\) >&2; exit 1");
  process.exit(0);
}
if (!refresh) {
  console.log("echo credentials.json has no refresh_token >&2; exit 1");
  process.exit(0);
}
const email = process.env.EMAIL;
const outDir = process.env.OUT_DIR;
fs.mkdirSync(outDir, { recursive: true, mode: 0o700 });
const outPath = path.join(outDir, `${email}.json`);
const payload = {
  token: '',
  refresh_token: refresh,
  token_uri: 'https://oauth2.googleapis.com/token',
  client_id: clientId,
  client_secret: clientSecret,
  scopes: [
    'https://www.googleapis.com/auth/gmail.modify',
    'https://www.googleapis.com/auth/drive',
    'https://www.googleapis.com/auth/calendar',
    'https://www.googleapis.com/auth/documents',
    'https://www.googleapis.com/auth/spreadsheets',
    'https://www.googleapis.com/auth/tasks',
    'https://www.googleapis.com/auth/contacts',
  ],
  expiry: '2000-01-01T00:00:00Z',
};
fs.writeFileSync(outPath, JSON.stringify(payload, null, 2) + '\n', { mode: 0o600 });
console.log(`OUT_PATH=${shQuote(outPath)}`);
console.log(`CLIENT_ID=${shQuote(clientId)}`);
console.log(`CLIENT_SECRET_VAL=${shQuote(clientSecret)}`);
JS
)"
else
  echo "Need python3 or node to parse JSON" >&2
  exit 1
fi

dotenv_set GOOGLE_OAUTH_CLIENT_ID "$CLIENT_ID"
dotenv_set GOOGLE_OAUTH_CLIENT_SECRET "$CLIENT_SECRET_VAL"
dotenv_set USER_GOOGLE_EMAIL "$EMAIL"

echo "Wrote $OUT_PATH"
echo "Updated .env GOOGLE_OAUTH_* + USER_GOOGLE_EMAIL (values not printed)"
echo "Then: make remote-deploy  (or make up)"
