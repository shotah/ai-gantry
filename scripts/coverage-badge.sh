#!/usr/bin/env bash
# Generate coverage.svg from coverage.out (Linux CI).
set -euo pipefail

PROFILE="${1:-coverage.out}"
OUT="${2:-coverage.svg}"

if [[ ! -f "$PROFILE" ]]; then
  echo "missing $PROFILE" >&2
  exit 1
fi

TOTAL=$(go tool cover -func="$PROFILE" | awk '/^total:/{print $3}' | tr -d '%')
if [[ -z "$TOTAL" ]]; then
  echo "could not parse total coverage" >&2
  exit 1
fi

# Color thresholds (match ucap / shields-style bands).
PCT=$(awk -v t="$TOTAL" 'BEGIN{printf "%.0f", t+0}')
COLOR="#e05d44"
if   [ "$PCT" -ge 80 ]; then COLOR="#4c1"
elif [ "$PCT" -ge 60 ]; then COLOR="#97ca00"
elif [ "$PCT" -ge 40 ]; then COLOR="#a4a61d"
fi

LABEL="coverage"
VALUE="${PCT}%"

mkdir -p "$(dirname "$OUT")"
cat >"$OUT" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="99" height="20" role="img" aria-label="${LABEL}: ${VALUE}">
  <title>${LABEL}: ${VALUE}</title>
  <linearGradient id="b" x2="0" y2="100%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <mask id="a"><rect width="99" height="20" rx="3" fill="#fff"/></mask>
  <g mask="url(#a)">
    <path fill="#555" d="M0 0h63v20H0z"/>
    <path fill="${COLOR}" d="M63 0h36v20H63z"/>
    <path fill="url(#b)" d="M0 0h99v20H0z"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
    <text x="31.5" y="15" fill="#010101" fill-opacity=".3">${LABEL}</text>
    <text x="31.5" y="14">${LABEL}</text>
    <text x="80" y="15" fill="#010101" fill-opacity=".3">${VALUE}</text>
    <text x="80" y="14">${VALUE}</text>
  </g>
</svg>
EOF

echo "wrote ${OUT} (${VALUE})"
