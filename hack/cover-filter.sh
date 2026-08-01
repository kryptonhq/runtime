#!/usr/bin/env bash
# Strip untestable and generated code from a Go coverage profile in place,
# so the reported percentage reflects hand-written, reachable code.
#
# Excluded:
#   zz_generated.deepcopy.go   controller-gen output; asserting on it
#                              measures the generator, not this project
#   cmd/**                     process wiring (flag parsing, manager setup);
#                              covered by e2e actually starting the binaries
#   examples/**                sample agent/MCP images, shipped separately
#   internal/controlplane/embed embedded UI assets, no statements of ours
#
# Usage: cover-filter.sh <profile.out> [output.out]
set -euo pipefail

IN=${1:?usage: cover-filter.sh <profile.out> [output.out]}
OUT=${2:-$IN}

if [[ ! -s "$IN" ]]; then
  echo "cover-filter: $IN is missing or empty; nothing to do" >&2
  exit 0
fi

EXCLUDE='zz_generated\.deepcopy\.go|/cmd/|/examples/|internal/controlplane/embed/'

tmp=$(mktemp)
# Line 1 is the `mode:` header and must survive verbatim.
head -1 "$IN" >"$tmp"
tail -n +2 "$IN" | { grep -Ev "$EXCLUDE" || true; } >>"$tmp"
mv "$tmp" "$OUT"
