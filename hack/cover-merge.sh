#!/usr/bin/env bash
# Merge the per-tier coverage profiles into coverage/merged.out and print
# the combined total.
#
# Go has no built-in profile merger. Concatenating profiles is valid as long
# as every profile uses the same mode and duplicate blocks get summed rather
# than repeated — which is what the awk pass below does. All tiers are built
# with -covermode=atomic so the counts are additive.
set -euo pipefail

COVER_DIR=${COVER_DIR:-coverage}
OUT="$COVER_DIR/merged.out"

shopt -s nullglob
profiles=("$COVER_DIR"/*.out)
# Don't fold a previous merge back into itself.
inputs=()
for p in "${profiles[@]}"; do
  [[ "$(basename "$p")" == "merged.out" ]] && continue
  [[ -s "$p" ]] && inputs+=("$p")
done

if [[ ${#inputs[@]} -eq 0 ]]; then
  echo "cover-merge: no profiles in $COVER_DIR; run a test tier first" >&2
  exit 1
fi

printf 'merging %d profile(s):\n' "${#inputs[@]}"
printf '  %s\n' "${inputs[@]}"

{
  echo "mode: atomic"
  # Sum the hit counts for identical coverage blocks across profiles.
  # Profile line format: <file>:<startLine>.<col>,<endLine>.<col> <numStmt> <count>
  awk 'FNR==1 { next }
       {
         key = $1 " " $2
         count[key] += $3
         order[key] = (key in order) ? order[key] : ++n
       }
       END {
         for (k in count) lines[order[k]] = k " " count[k]
         for (i = 1; i <= n; i++) print lines[i]
       }' "${inputs[@]}"
} >"$OUT"

echo
go tool cover -func="$OUT" | tail -1
