#!/usr/bin/env bash
# Copy legacy secrets/<tool> trees into data/.config/<tool> (Docker + native layout).
# Idempotent: does not overwrite existing destination files.
# Usage: make secrets-migrate
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

copy_tree() {
  local src="$1" dst="$2"
  if [[ ! -d "$src" ]]; then
    echo "skip $src (missing)"
    return 0
  fi
  mkdir -p "$dst"
  local n=0
  while IFS= read -r -d '' f; do
    local rel="${f#"$src"/}"
    case "$rel" in
      .gitkeep|.gitignore|*/.gitkeep|*/.gitignore) continue ;;
    esac
    local target="$dst/$rel"
    mkdir -p "$(dirname "$target")"
    if [[ -e "$target" ]]; then
      echo "keep existing $dst/$rel"
      continue
    fi
    cp -a "$f" "$target"
    echo "copied $src/$rel -> $dst/$rel"
    n=$((n + 1))
  done < <(find "$src" -type f -print0)
  COPIED=$((COPIED + n))
}

COPIED=0
copy_tree secrets/google-mcp data/.config/google-mcp
copy_tree secrets/strava data/.config/strava
copy_tree secrets/garmin data/.config/garmin
copy_tree secrets/youtube data/.config/youtube
copy_tree secrets/ytmusic data/.config/youtube

echo
echo "Migrated ${COPIED} file(s) into data/.config/."
echo "Docker and native both use that tree. Push with: make secrets-sync"
echo "Legacy secrets/* can stay as backup until you are happy, then delete."
