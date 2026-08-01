#!/usr/bin/env bash
# internal/controlplane/embed go:embeds dist/, so the directory needs at
# least one non-.gitkeep file for the package to compile. Jobs that only
# lint or test Go don't need the real bundle — this writes a stub so they
# can skip `make ui` (pnpm install + vite build) entirely.
#
# The stub is a minimal but *structurally faithful* vite bundle: an
# index.html with the #root mount point plus a favicon.svg. ui_test.go
# exercises UI() — asset-first serving, SPA fallback, content-type — and
# uses those two files as its fixtures, so a stub missing them turns those
# tests red for reasons that have nothing to do with the handler. What the
# stub deliberately does NOT prove is that vite's real output is embedded
# and served; the e2e tier builds real images and covers that.
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
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <link rel="icon" type="image/svg+xml" href="/ui/favicon.svg" />
    <title>Krypton (stub build)</title>
  </head>
  <body>
    <div id="root"></div>
    <p>This is a CI stub. Run <code>make ui</code> for the real operator UI.</p>
  </body>
</html>
HTML

cat >"$DIST/favicon.svg" <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="#0f172a"/></svg>
SVG

echo "▸ wrote stub $DIST/index.html and $DIST/favicon.svg"
