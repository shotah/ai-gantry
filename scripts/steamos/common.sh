# shellcheck shell=bash
# Shared by scripts in this directory. Sourced, not executed.

steamos_arch() {
	case "$(uname -m)" in
	x86_64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*)
		echo "unsupported arch: $(uname -m)" >&2
		return 1
		;;
	esac
}

steamos_config_dir() {
	echo "${XDG_CONFIG_HOME:-$HOME/.config}/steamos-dev"
}

steamos_go_root() {
	echo "${STEAMOS_GO_PREFIX:-$HOME/.local}/go"
}

steamos_read_go_version() {
	if [[ -n "${GO_VERSION:-}" ]]; then
		echo "$GO_VERSION"
		return 0
	fi
	local pin cfg repo
	cfg="$(steamos_config_dir)/go-version"
	if [[ -f "$cfg" ]]; then
		tr -d '[:space:]' <"$cfg"
		return 0
	fi
	repo=""
	if [[ -n "${STEAMOS_GANTRY_ROOT:-}" ]]; then
		repo="$STEAMOS_GANTRY_ROOT"
	else
		local kit
		kit="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
		if [[ -f "$kit/../../go.mod" ]]; then
			repo="$(cd "$kit/../.." && pwd)"
		fi
	fi
	if [[ -n "$repo" && -f "$repo/go.mod" ]]; then
		awk '/^go / { print $2; exit }' "$repo/go.mod"
		return 0
	fi
	echo "1.26.0"
}

steamos_path_prefix() {
	printf '%s:%s:%s' "$(steamos_go_root)/bin" "$HOME/go/bin" "$HOME/.local/bin"
}
