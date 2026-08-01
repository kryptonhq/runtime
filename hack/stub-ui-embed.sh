#!/usr/bin/env bash
# internal/controlplane/embed go:embeds dist/, so the directory needs at
# least one non-.gitkeep file for the package to compile. Jobs that only
# lint or test Go don't need the real bundle — this writes a stub so they
# can skip `make ui` (pnpm install + vite build) entirely.
#
# No-op when a real build is already staged.
set -euo pipefail

DIST=internal/controlplane/embed/dist

if [[ -f "$DIST/index.html" ]]; then
  echo "▸ $DIST/index.html already present; leaving it alone"
  exit 0
fi

mkdir -p "$DIST"
cat >"$DIST/index.html" <<'HTML'
<!doctype html>
<title>Krypton (stub build)</title>
<p>This is a CI stub. Run <code>make ui</code> for the real operator UI.</p>
HTML
echo "▸ wrote stub $DIST/index.html"
