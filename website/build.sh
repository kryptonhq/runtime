#!/usr/bin/env bash
#
# Builds the Krypton docs site on Vercel. Adapted from
# https://gohugo.io/host-and-deploy/host-on-vercel/.
#
# Vercel's Hugo runtime image doesn't include Go, so `hugo mod get`
# fails when the site uses modules (we pull in Docsy via
# github.com/google/docsy). This script installs Go + Hugo into a
# user-local prefix and prepends them to PATH before invoking Hugo.
#
# Vercel installs Node automatically from package.json so we don't
# need to fetch it here.

set -euo pipefail

GO_VERSION=1.25.0
HUGO_VERSION=0.161.1

tmp=$(mktemp -d)
trap 'rm -rf "${tmp}"' EXIT SIGINT SIGTERM

mkdir -p "${HOME}/.local"

echo "Installing Go ${GO_VERSION}..."
curl -sSLo "${tmp}/go.tar.gz" \
  "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
tar -C "${HOME}/.local" -xf "${tmp}/go.tar.gz"
export PATH="${HOME}/.local/go/bin:${PATH}"

echo "Installing Hugo extended ${HUGO_VERSION}..."
curl -sSLo "${tmp}/hugo.tar.gz" \
  "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-amd64.tar.gz"
mkdir -p "${HOME}/.local/hugo"
tar -C "${HOME}/.local/hugo" -xf "${tmp}/hugo.tar.gz"
export PATH="${HOME}/.local/hugo:${PATH}"

echo "Versions:"
go version
hugo version

# Hugo's enableGitInfo wants full history. Vercel does a shallow
# clone with a narrow refspec (refs/heads/main only) and NO tag
# refs, so a plain `git fetch --tags` is a silent no-op. Add the
# tag refspec explicitly and force-fetch.
git config core.quotepath false
git config --add remote.origin.fetch '+refs/tags/*:refs/tags/*' 2>/dev/null || true
if [ "$(git rev-parse --is-shallow-repository)" = "true" ]; then
  git fetch --unshallow --force origin '+refs/heads/*:refs/remotes/origin/*' '+refs/tags/*:refs/tags/*'
else
  git fetch --force origin '+refs/tags/*:refs/tags/*'
fi
echo "Tags visible to git:"
git tag --list --sort=-v:refname | head -5
echo "git describe says: $(git describe --tags --abbrev=0 2>&1 || echo '<none>')"

echo "Installing npm deps..."
npm install

# Stamp the latest git tag into HUGO_PARAMS_KRYPTONVERSION. Hugo
# automatically maps HUGO_PARAMS_<NAME> env vars to
# .Site.Params.<name>, so docs shortcodes ({{< version >}} etc.)
# and the navbar badge resolve to the current release with no commit
# required. Falls back to "main" if no tags exist (fresh repo).
KRYPTON_VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "main")
export HUGO_PARAMS_KRYPTONVERSION="${KRYPTON_VERSION}"
echo "Building site at version ${KRYPTON_VERSION}..."

hugo --gc --minify
