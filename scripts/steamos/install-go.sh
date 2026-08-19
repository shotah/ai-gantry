#!/usr/bin/env bash
# Install the official Go tarball into $HOME/.local/go (survives SteamOS A/B).
set -euo pipefail

_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$_here/common.sh" ]]; then
	# shellcheck source=common.sh
	source "$_here/common.sh"
elif [[ -f "${HOME}/.local/lib/steamos/common.sh" ]]; then
	# shellcheck source=/dev/null
	source "${HOME}/.local/lib/steamos/common.sh"
else
	echo "steamos-install-go: missing common.sh" >&2
	exit 1
fi

need() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "need $1 on PATH" >&2
		exit 1
	}
}

need curl
need tar
need sha256sum
need python3

ver="$(steamos_read_go_version)"
ver="${ver#go}"
arch="$(steamos_arch)"
filename="go${ver}.linux-${arch}.tar.gz"
prefix="${STEAMOS_GO_PREFIX:-$HOME/.local}"
dest="$prefix/go"

echo "Go $ver ($arch) → $dest"

meta="$(python3 - "$ver" "$arch" "$filename" <<'PY'
import json, sys, urllib.request
ver, arch, filename = sys.argv[1], sys.argv[2], sys.argv[3]
url = "https://go.dev/dl/?mode=json&include=all"
with urllib.request.urlopen(url, timeout=30) as r:
    releases = json.load(r)
want = "go" + ver
for rel in releases:
    if rel.get("version") != want:
        continue
    for f in rel.get("files") or []:
        if f.get("filename") == filename and f.get("kind") == "archive":
            sha = f.get("sha256") or ""
            file_url = f.get("url") or ("https://go.dev/dl/" + filename)
            print(sha)
            print(file_url)
            sys.exit(0)
sys.stderr.write("no archive on go.dev for %s / %s\n" % (want, filename))
sys.exit(1)
PY
)"

sha="$(printf '%s\n' "$meta" | sed -n '1p')"
url="$(printf '%s\n' "$meta" | sed -n '2p')"
if [[ -z "$sha" || -z "$url" ]]; then
	echo "could not resolve $filename from go.dev" >&2
	exit 1
fi

if [[ -x "$dest/bin/go" ]]; then
	have="$("$dest/bin/go" env GOVERSION 2>/dev/null || true)"
	have="${have#go}"
	if [[ "$have" == "$ver" ]]; then
		echo "already $have at $dest"
		mkdir -p "$(steamos_config_dir)"
		printf '%s\n' "$ver" >"$(steamos_config_dir)/go-version"
		exit 0
	fi
	echo "replacing $have → $ver"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
echo "download $url"
curl -fsSL --retry 3 -o "$tmp/go.tgz" "$url"
printf '%s  %s\n' "$sha" "$tmp/go.tgz" | sha256sum -c -
tar -C "$tmp" -xzf "$tmp/go.tgz"
if [[ ! -x "$tmp/go/bin/go" ]]; then
	echo "tarball did not contain go/bin/go" >&2
	exit 1
fi

mkdir -p "$prefix"
if [[ -e "$dest" ]]; then
	rm -rf "$dest.old"
	mv "$dest" "$dest.old"
fi
mv "$tmp/go" "$dest"
rm -rf "$dest.old"

mkdir -p "$(steamos_config_dir)"
printf '%s\n' "$ver" >"$(steamos_config_dir)/go-version"

echo "installed $($dest/bin/go version)"
echo "next: open a new terminal, or run:  export PATH=\"$(steamos_path_prefix):\$PATH\""
