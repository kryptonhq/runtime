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

# Hugo's enableGitInfo wants full history.
git config core.quotepath false
if [ "$(git rev-parse --is-shallow-repository)" = "true" ]; then
  git fetch --unshallow
fi

echo "Installing npm deps..."
npm install

echo "Building site..."
hugo --gc --minify
