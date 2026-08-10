#!/usr/bin/env bash
# Install this consumer repo onto /opt/gantry and enable systemd.
# Run from the repo root on the Linux host: sudo ./install.sh
set -euo pipefail

DEST=/opt/gantry
REPO_OWNER="${REPO_OWNER:-shotah}"
REPO_NAME="${REPO_NAME:-ai-gantry}"
# Empty = latest GitHub release tag; or pin e.g. GANTRY_VERSION=0.1.0
GANTRY_VERSION="${GANTRY_VERSION:-}"

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ ! -f "$SCRIPT_DIR/gantry.service" ]; then
  echo "Run from the consumer repo root (missing gantry.service)" >&2
  exit 1
fi

if ! id -u gantry >/dev/null 2>&1; then
  useradd --system --home "$DEST/data" --shell /usr/sbin/nologin gantry
fi

mkdir -p "$DEST/bin" "$DEST/data" "$DEST/persona"

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *)
    echo "Unsupported arch: $arch" >&2
    exit 1
    ;;
esac
os=linux

if [ -z "$GANTRY_VERSION" ]; then
  echo "Resolving latest release…"
  GANTRY_VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
  GANTRY_VERSION="${GANTRY_VERSION#v}"
fi
if [ -z "$GANTRY_VERSION" ]; then
  echo "Could not resolve GANTRY_VERSION (set it explicitly)" >&2
  exit 1
fi

asset="gantry_${GANTRY_VERSION}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/v${GANTRY_VERSION}/${asset}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $url"
curl -fsSL -o "$tmp/$asset" "$url"
tar -xzf "$tmp/$asset" -C "$tmp"
bin="$(find "$tmp" -type f -name gantry | head -n1)"
if [ -z "$bin" ] || [ ! -x "$bin" ]; then
  # tarball may ship non-executable bit; still accept a file named gantry
  bin="$(find "$tmp" -type f -name gantry | head -n1)"
fi
if [ -z "$bin" ]; then
  echo "gantry binary not found in archive" >&2
  exit 1
fi
install -m 0755 "$bin" "$DEST/gantry"

if [ -f "$SCRIPT_DIR/gantry.env" ]; then
  install -m 0640 -o gantry -g gantry "$SCRIPT_DIR/gantry.env" "$DEST/gantry.env"
elif [ -f "$SCRIPT_DIR/gantry.env.example" ] && [ ! -f "$DEST/gantry.env" ]; then
  install -m 0640 -o gantry -g gantry "$SCRIPT_DIR/gantry.env.example" "$DEST/gantry.env"
  echo "Wrote $DEST/gantry.env from example — edit secrets before relying on the bot"
fi

if [ -f "$SCRIPT_DIR/mcp.toml" ]; then
  install -m 0644 -o gantry -g gantry "$SCRIPT_DIR/mcp.toml" "$DEST/mcp.toml"
fi

if compgen -G "$SCRIPT_DIR/persona/*.md" >/dev/null 2>&1; then
  install -m 0644 -o gantry -g gantry "$SCRIPT_DIR"/persona/*.md "$DEST/persona/"
fi

chown -R gantry:gantry "$DEST/data" "$DEST/persona"
chmod 0750 "$DEST/data"

install -m 0644 "$SCRIPT_DIR/gantry.service" /etc/systemd/system/gantry.service
systemctl daemon-reload
systemctl enable gantry.service
systemctl restart gantry.service

echo "Installed. Logs: journalctl -u gantry -f"
echo "Heartbeat: sudo -u gantry $DEST/gantry status"
