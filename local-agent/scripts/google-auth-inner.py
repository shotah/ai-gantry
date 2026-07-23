#!/usr/bin/env python3
"""Interactive Google OAuth for magks/google-workspace-mcp-go credentials.

Runs in a throwaway container (make google-auth). Listens on a fixed port,
prints an auth URL, exchanges the code, and writes:
  $WORKSPACE_MCP_CREDENTIALS_DIR/<email>.json
in the MCP on-disk format (same as start_google_auth).

No gws CLI required.
"""
from __future__ import annotations

import json
import os
import secrets
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer

SCOPES = [
    "https://www.googleapis.com/auth/gmail.modify",
    "https://www.googleapis.com/auth/drive",
    "https://www.googleapis.com/auth/calendar",
    "https://www.googleapis.com/auth/documents",
    "https://www.googleapis.com/auth/spreadsheets",
    "https://www.googleapis.com/auth/tasks",
    "https://www.googleapis.com/auth/contacts",
]

PORT = int(os.environ.get("GOOGLE_AUTH_PORT", "4100"))
REDIRECT = f"http://localhost:{PORT}/oauth2callback"
CRED_DIR = os.environ.get(
    "WORKSPACE_MCP_CREDENTIALS_DIR", "/data/credentials"
)


def die(msg: str) -> None:
    print(f"Error: {msg}", file=sys.stderr)
    sys.exit(1)


def main() -> None:
    client_id = os.environ.get("GOOGLE_OAUTH_CLIENT_ID", "").strip()
    client_secret = os.environ.get("GOOGLE_OAUTH_CLIENT_SECRET", "").strip()
    email_hint = os.environ.get("USER_GOOGLE_EMAIL", "").strip()
    if not client_id or not client_secret:
        die("Set GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET in .env")
    if not email_hint:
        die("Set USER_GOOGLE_EMAIL in .env (e.g. you@gmail.com)")

    os.makedirs(CRED_DIR, mode=0o700, exist_ok=True)
    out_path = os.path.join(CRED_DIR, f"{email_hint}.json")
    if os.path.isfile(out_path):
        os.remove(out_path)
        print(f"Cleared stale credential: {out_path}", file=sys.stderr)

    state = secrets.token_hex(16)
    result: dict[str, str] = {}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, fmt: str, *args) -> None:  # noqa: A003
            return

        def do_GET(self) -> None:  # noqa: N802
            parsed = urllib.parse.urlparse(self.path)
            if parsed.path != "/oauth2callback":
                self.send_response(404)
                self.end_headers()
                return
            q = urllib.parse.parse_qs(parsed.query)
            if q.get("error"):
                result["error"] = q["error"][0]
                body = b"<h1>Auth failed</h1><p>You can close this tab.</p>"
            elif q.get("state", [""])[0] != state:
                result["error"] = "state mismatch"
                body = b"<h1>State mismatch</h1><p>You can close this tab.</p>"
            elif not q.get("code"):
                result["error"] = "missing code"
                body = b"<h1>Missing code</h1><p>You can close this tab.</p>"
            else:
                result["code"] = q["code"][0]
                body = (
                    b"<h1>Google Workspace authorized</h1>"
                    b"<p>You can close this tab and return to the terminal.</p>"
                )
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    server = HTTPServer(("0.0.0.0", PORT), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    params = {
        "client_id": client_id,
        "redirect_uri": REDIRECT,
        "response_type": "code",
        "scope": " ".join(SCOPES),
        "access_type": "offline",
        "prompt": "consent",
        "state": state,
        "login_hint": email_hint,
    }
    auth_url = "https://accounts.google.com/o/oauth2/v2/auth?" + urllib.parse.urlencode(
        params
    )

    print("", file=sys.stderr)
    print("Open this URL in your browser, approve access, then return here:", file=sys.stderr)
    print("", file=sys.stderr)
    print(auth_url, file=sys.stderr)
    print("", file=sys.stderr)
    print(f"Waiting for OAuth callback on {REDIRECT} (5 minute timeout)...", file=sys.stderr)

    deadline = time.time() + 300
    while time.time() < deadline and "code" not in result and "error" not in result:
        time.sleep(0.2)
    server.shutdown()

    if "error" in result:
        die(result["error"])
    if "code" not in result:
        die("timed out waiting for OAuth callback")

    token = exchange_code(client_id, client_secret, result["code"])
    email = fetch_email(token.get("access_token", "")) or email_hint

    # Prefer the consented account if it differs from the hint.
    out_path = os.path.join(CRED_DIR, f"{email}.json")
    payload = {
        "token": token.get("access_token", ""),
        "refresh_token": token.get("refresh_token", ""),
        "token_uri": "https://oauth2.googleapis.com/token",
        "client_id": client_id,
        "client_secret": client_secret,
        "scopes": SCOPES,
        "expiry": "",
    }
    expires_in = token.get("expires_in")
    if expires_in:
        # Store a past-ish expiry so the MCP refreshes soon if clocks drift;
        # real expiry is handled via refresh_token.
        from datetime import datetime, timedelta, timezone

        exp = datetime.now(timezone.utc) + timedelta(seconds=int(expires_in))
        payload["expiry"] = exp.strftime("%Y-%m-%dT%H:%M:%SZ")

    if not payload["refresh_token"]:
        die(
            "Google did not return a refresh_token. Revoke prior access at "
            "https://myaccount.google.com/permissions and retry with prompt=consent."
        )

    with open(out_path, "w", encoding="utf-8", newline="\n") as f:
        json.dump(payload, f, indent=2)
        f.write("\n")
    try:
        os.chmod(out_path, 0o600)
    except OSError:
        pass

    print(f"Login successful. Wrote {out_path}", file=sys.stderr)
    print("Then: make remote-deploy  (or make up)", file=sys.stderr)


def exchange_code(client_id: str, client_secret: str, code: str) -> dict:
    data = urllib.parse.urlencode(
        {
            "code": code,
            "client_id": client_id,
            "client_secret": client_secret,
            "redirect_uri": REDIRECT,
            "grant_type": "authorization_code",
        }
    ).encode()
    req = urllib.request.Request(
        "https://oauth2.googleapis.com/token",
        data=data,
        method="POST",
        headers={"Content-Type": "application/x-www-form-urlencoded"},
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")
        die(f"token exchange failed: {e.code} {body}")
        return {}


def fetch_email(access_token: str) -> str:
    if not access_token:
        return ""
    req = urllib.request.Request(
        "https://www.googleapis.com/oauth2/v2/userinfo",
        headers={"Authorization": f"Bearer {access_token}"},
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            info = json.loads(resp.read().decode())
            return str(info.get("email") or "")
    except Exception:
        return ""


if __name__ == "__main__":
    main()
