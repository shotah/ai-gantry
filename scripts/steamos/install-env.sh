#!/usr/bin/env bash
# PATH + login guard so GUI apps (Konsole, Cursor) see ~/.local/go after SteamOS updates.
set -euo pipefail

_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$_here/common.sh" ]]; then
	# shellcheck source=common.sh
	source "$_here/common.sh"
elif [[ -f "${HOME}/.local/lib/steamos/common.sh" ]]; then
	# shellcheck source=/dev/null
	source "${HOME}/.local/lib/steamos/common.sh"
else
	echo "steamos-install-env: missing common.sh" >&2
	exit 1
fi

lib="$HOME/.local/lib/steamos"
bin="$HOME/.local/bin"
envd="${XDG_CONFIG_HOME:-$HOME/.config}/environment.d"
plasma="${XDG_CONFIG_HOME:-$HOME/.config}/plasma-workspace/env"
unitdir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
bashrc="$HOME/.bashrc"
marker="# steamos-dev PATH (ai-gantry scripts/steamos)"
cgo_marker="# steamos-dev CGO_ENABLED=0 (ai-gantry scripts/steamos)"

mkdir -p "$lib" "$bin" "$envd" "$plasma" "$unitdir" "$(steamos_config_dir)"

install -m 0644 "$_here/common.sh" "$lib/common.sh"
for s in install-go.sh install-env.sh doctor.sh guard.sh bootstrap.sh; do
	if [[ -f "$_here/$s" ]]; then
		name="steamos-${s%.sh}"
		[[ "$s" == "install-go.sh" ]] && name="steamos-install-go"
		[[ "$s" == "install-env.sh" ]] && name="steamos-install-env"
		[[ "$s" == "doctor.sh" ]] && name="steamos-doctor"
		[[ "$s" == "guard.sh" ]] && name="steamos-go-guard"
		[[ "$s" == "bootstrap.sh" ]] && name="steamos-bootstrap"
		install -m 0755 "$_here/$s" "$bin/$name"
	fi
done

path="$(steamos_path_prefix)"
cat >"$envd/99-steamos-dev.conf" <<EOF
# Managed by scripts/steamos/install-env.sh — Go + go-install tools in \$HOME.
PATH=${path}:/usr/local/bin:/usr/bin
CGO_ENABLED=0
EOF

cat >"$plasma/steamos-dev.sh" <<EOF
# Managed by scripts/steamos/install-env.sh
export PATH="${path}:\$PATH"
export CGO_ENABLED=0
EOF
chmod +x "$plasma/steamos-dev.sh"

if [[ ! -f "$bashrc" ]]; then
	touch "$bashrc"
fi
if ! grep -Fqx "$marker" "$bashrc"; then
	cat >>"$bashrc" <<EOF

$marker
export PATH="${path}:\$PATH"
EOF
	echo "appended PATH to $bashrc"
else
	echo "bashrc already has steamos-dev PATH"
fi
if ! grep -Fqx "$cgo_marker" "$bashrc"; then
	cat >>"$bashrc" <<EOF

$cgo_marker
export CGO_ENABLED=0
EOF
	echo "appended CGO_ENABLED=0 to $bashrc"
fi

cat >"$unitdir/steamos-go-guard.service" <<EOF
[Unit]
Description=Reinstall \$HOME/.local/go if missing after a SteamOS update
After=default.target

[Service]
Type=oneshot
ExecStart=%h/.local/bin/steamos-go-guard

[Install]
WantedBy=default.target
EOF

if command -v systemctl >/dev/null 2>&1; then
	systemctl --user daemon-reload
	systemctl --user enable steamos-go-guard.service
	echo "enabled systemd --user steamos-go-guard.service"
else
	echo "systemctl not available; skip user unit" >&2
fi

echo "env installed. Log out of Desktop (or reboot) so Cursor/Konsole inherit PATH."
