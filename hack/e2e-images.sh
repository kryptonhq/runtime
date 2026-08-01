#!/usr/bin/env bash
# Builds the images the e2e suite needs, and optionally saves them into a
# single tar.
#
# Split out of hack/e2e.sh so the release matrix can build once and hand the
# same archive to every Kubernetes version, instead of each leg spending ~5
# minutes rebuilding byte-identical images.
#
# Environment:
#   REGISTRY=krypton            image namespace
#   TAG=e2e                     image tag
#   SAVE_ARCHIVE=<path>         also `docker save` every image into <path>
#   DEPLOY_LLM=true             pull llama.cpp and include it in the archive
#   IMAGE_LIST=<path>           where to write the built image refs, one per
#                               line (hack/e2e.sh reads this to load them)
set -euo pipefail

TAG=${TAG:-e2e}
REGISTRY=${REGISTRY:-krypton}
DEPLOY_LLM=${DEPLOY_LLM:-false}
SAVE_ARCHIVE=${SAVE_ARCHIVE:-}
LLAMA_CPP_IMAGE=${LLAMA_CPP_IMAGE:-ghcr.io/ggml-org/llama.cpp:server}

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
IMAGE_LIST=${IMAGE_LIST:-$REPO_ROOT/.e2e-images.txt}

note() { printf '\n\033[36m▸ %s\033[0m\n' "$*"; }

# Components built from the root Dockerfile via --build-arg COMPONENT.
COMPONENTS=(manager control-plane gateway krypton-proxy mcp-stdio-bridge)

note "build UI (embedded into the control-plane image)"
if command -v pnpm >/dev/null 2>&1; then
  ( cd ui && pnpm install --frozen-lockfile && pnpm build )
  rm -rf internal/controlplane/embed/dist
  mkdir -p internal/controlplane/embed/dist
  cp -R ui/dist/. internal/controlplane/embed/dist/
  touch internal/controlplane/embed/dist/.gitkeep
else
  echo "! pnpm not found; using a stub UI bundle"
  ./hack/stub-ui-embed.sh
fi

note "build images"
: >"$IMAGE_LIST"
for binary in "${COMPONENTS[@]}"; do
  docker build --quiet --build-arg COMPONENT="$binary" \
               -t "${REGISTRY}/${binary}:${TAG}" . >/dev/null
  echo "  built ${REGISTRY}/${binary}:${TAG}"
  echo "${REGISTRY}/${binary}:${TAG}" >>"$IMAGE_LIST"
done

docker build --quiet -f examples/mcp/go/Dockerfile \
             -t "${REGISTRY}/mcp-hello:${TAG}" . >/dev/null
echo "  built ${REGISTRY}/mcp-hello:${TAG}"
echo "${REGISTRY}/mcp-hello:${TAG}" >>"$IMAGE_LIST"

if [[ "$DEPLOY_LLM" == "true" ]]; then
  note "pull llama.cpp image"
  docker pull "$LLAMA_CPP_IMAGE"
  echo "$LLAMA_CPP_IMAGE" >>"$IMAGE_LIST"
fi

if [[ -n "$SAVE_ARCHIVE" ]]; then
  note "save images to $SAVE_ARCHIVE"
  mkdir -p "$(dirname "$SAVE_ARCHIVE")"
  # One archive for every image: `kind load image-archive` accepts a
  # multi-image tar, so the matrix legs need a single download and a
  # single load call.
  # shellcheck disable=SC2046
  docker save -o "$SAVE_ARCHIVE" $(tr '\n' ' ' <"$IMAGE_LIST")
  printf '  %s (%s)\n' "$SAVE_ARCHIVE" "$(du -h "$SAVE_ARCHIVE" | cut -f1)"
fi
