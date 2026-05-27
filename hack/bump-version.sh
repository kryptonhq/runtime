#!/usr/bin/env bash
#
# Bump the published Krypton version everywhere it's hardcoded.
#
# Usage:
#   ./hack/bump-version.sh v0.2.0
#
# Updates:
#   - website/hugo.yaml          params.kryptonVersion (drives docs shortcodes)
#   - README.md                  helm install snippet
#
# Run BEFORE pushing the matching git tag so that the release-images,
# release-examples, and release-chart workflows produce artifacts that
# the docs already reference.

set -euo pipefail

new=${1:?"usage: $0 vX.Y.Z"}
[[ "$new" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$ ]] \
  || { echo "version must be vX.Y.Z[-suffix], got: $new" >&2; exit 1; }

bare=${new#v}

# website/hugo.yaml
sed -i.bak -E "s|(  kryptonVersion: ).*|\1${new}|" website/hugo.yaml
rm website/hugo.yaml.bak

# README.md
sed -i.bak -E "s|(--version )[0-9.]+(-[A-Za-z0-9.]+)?|\1${bare}|" README.md
sed -i.bak -E "s|(image.tag=)v[0-9.]+(-[A-Za-z0-9.]+)?|\1${new}|" README.md
rm README.md.bak

echo "Bumped to ${new}:"
git diff --stat website/hugo.yaml README.md

cat <<EOF

Next:
  git add website/hugo.yaml README.md
  git commit -m "chore: bump version to ${new}"
  git push
  git tag ${new}
  git push origin ${new}
EOF
