#!/usr/bin/env bash
# Report whether the Deck still has a home-local Go toolchain after an OS update.
set -euo pipefail

_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$_here/common.sh" ]]; then
	# shellcheck source=common.sh
	source "$_here/common.sh"
elif [[ -f "${HOME}/.local/lib/steamos/common.sh" ]]; then
	# shellcheck source=/dev/null
	source "${HOME}/.local/lib/steamos/common.sh"
else
	echo "steamos-doctor: missing common.sh" >&2
	exit 1
fi

ok=0
warn() { printf 'WARN  %s\n' "$*"; }
fail() { printf 'FAIL  %s\n' "$*"; ok=1; }
pass() { printf 'OK    %s\n' "$*"; }

go_home="$(steamos_go_root)/bin/go"
if [[ -x "$go_home" ]]; then
	pass "home Go: $($go_home version) ($go_home)"
else
	fail "no $go_home — run: ./scripts/steamos/install-go.sh"
fi

if command -v go >/dev/null 2>&1; then
	resolved="$(command -v go)"
	pass "PATH go: $resolved ($(go version 2>/dev/null | head -1))"
	if [[ -x "$go_home" && "$resolved" != "$go_home" ]]; then
		warn "PATH go is not ~/.local/go (host pacman?). Prefer $go_home — host /usr will vanish on update."
	fi
else
	fail "go not on PATH. New terminal after install-env.sh, or: export PATH=\"$(steamos_path_prefix):\$PATH\""
fi

if command -v make >/dev/null 2>&1; then
	pass "make: $(command -v make) ($(make --version 2>/dev/null | head -1))"
else
	warn "make not on PATH (usually on the SteamOS image). Do not pacman -S it on the host."
fi

if command -v git >/dev/null 2>&1; then
	pass "git: $(command -v git)"
else
	fail "git missing"
fi

unit="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/steamos-go-guard.service"
if [[ -f "$unit" ]]; then
	if systemctl --user is-enabled steamos-go-guard.service >/dev/null 2>&1; then
		pass "login guard enabled (steamos-go-guard.service)"
	else
		warn "guard unit present but not enabled — run install-env.sh"
	fi
else
	warn "no login guard — run install-env.sh"
fi

envf="${XDG_CONFIG_HOME:-$HOME/.config}/environment.d/99-steamos-dev.conf"
if [[ -f "$envf" ]]; then
	pass "environment.d PATH file present"
else
	warn "no $envf — GUI apps may not see Go until install-env.sh"
fi

exit "$ok"
