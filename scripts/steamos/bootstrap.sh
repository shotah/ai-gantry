#!/usr/bin/env bash
# One shot: PATH + login guard + official Go in $HOME/.local/go.
set -euo pipefail

_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$_here"

./install-env.sh
./install-go.sh
# This shell has not logged out yet — still inject PATH for doctor.
# shellcheck source=common.sh
source "$_here/common.sh"
export PATH="$(steamos_path_prefix):${PATH}"
./doctor.sh || true

echo
echo "Done. Log out of Desktop (or reboot), then:  go version && steamos-doctor"
