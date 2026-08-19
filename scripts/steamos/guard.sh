#!/usr/bin/env bash
# Reinstall ~/.local/go when a SteamOS update (or a bad PATH) left it missing.
set -euo pipefail

_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$_here/common.sh" ]]; then
	# shellcheck source=common.sh
	source "$_here/common.sh"
elif [[ -f "${HOME}/.local/lib/steamos/common.sh" ]]; then
	# shellcheck source=/dev/null
	source "${HOME}/.local/lib/steamos/common.sh"
else
	echo "steamos-go-guard: missing common.sh" >&2
	exit 1
fi

go_bin="$(steamos_go_root)/bin/go"
installer="$HOME/.local/bin/steamos-install-go"
if [[ ! -x "$installer" && -x "$_here/install-go.sh" ]]; then
	installer="$_here/install-go.sh"
fi

if [[ -x "$go_bin" ]]; then
	exit 0
fi

if [[ ! -x "$installer" ]]; then
	echo "steamos-go-guard: Go missing and no installer at $installer" >&2
	exit 1
fi

echo "steamos-go-guard: $go_bin missing — running installer"
exec "$installer"
