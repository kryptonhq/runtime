#!/usr/bin/env bash
# End-to-end suite against a real kind cluster.
#
# Two layers, because they're good at different things:
#
#   1. Chainsaw (test/e2e/chainsaw)  declarative "apply this CR, assert these
#      objects exist with these fields". Replaces hundreds of lines of Go for
#      reconcile assertions.
#   2. Go (test/e2e)                 behavioural assertions YAML can't express:
#      cold-start latency, streaming, OpenAI-compat routes, scale-to-zero.
#
# Environment:
#   CLUSTER=krypton-e2e         kind cluster name
#   SKIP_CLUSTER_CREATE=true    reuse an existing cluster (CI creates its own)
#   SKIP_BUILD=true             reuse already-loaded images
#   DEPLOY_LLM=true             also run the llama.cpp/Qwen model path (slow)
#   KEEP_CLUSTER=true           don't delete the cluster on exit
set -euo pipefail

CLUSTER=${CLUSTER:-krypton-e2e}
NAMESPACE=${NAMESPACE:-krypton-system}
RELEASE=${RELEASE:-krypton}
TAG=${TAG:-e2e}
REGISTRY=${REGISTRY:-krypton}
DEPLOY_LLM=${DEPLOY_LLM:-false}
SKIP_CLUSTER_CREATE=${SKIP_CLUSTER_CREATE:-false}
SKIP_BUILD=${SKIP_BUILD:-false}
KEEP_CLUSTER=${KEEP_CLUSTER:-false}
LLAMA_CPP_IMAGE=${LLAMA_CPP_IMAGE:-ghcr.io/ggml-org/llama.cpp:server}

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

note()  { printf '\n\033[36m▸ %s\033[0m\n' "$*"; }
fatal() { printf '\033[31m✖ %s\033[0m\n' "$*" >&2; exit 1; }
ok()    { printf '\033[32m✓ %s\033[0m\n' "$*"; }

require() { command -v "$1" >/dev/null 2>&1 || fatal "missing required tool: $1"; }
require kubectl
require helm
require docker

cleanup() {
  local code=$?
  if [[ $code -ne 0 ]]; then
    printf '\n\033[31m✖ e2e failed; collecting diagnostics\033[0m\n' >&2
    ./hack/e2e-diagnostics.sh || true
  fi
  # Kill any port-forwards we started.
  if [[ -n "${PF_PIDS:-}" ]]; then
    # shellcheck disable=SC2086
    kill $PF_PIDS 2>/dev/null || true
  fi
  if [[ "$KEEP_CLUSTER" != "true" && "$SKIP_CLUSTER_CREATE" != "true" ]]; then
    kind delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
  fi
  exit $code
}
trap cleanup EXIT

# ---- cluster ---------------------------------------------------------------

if [[ "$SKIP_CLUSTER_CREATE" != "true" ]]; then
  require kind
  note "kind cluster: $CLUSTER"
  if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
    kind create cluster --name "$CLUSTER" --config hack/kind.yaml
  fi
fi
kubectl config use-context "kind-${CLUSTER}" >/dev/null

# ---- build + load ----------------------------------------------------------

if [[ "$SKIP_BUILD" != "true" ]]; then
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
  for binary in manager control-plane gateway krypton-proxy mcp-stdio-bridge; do
    docker build --quiet --build-arg COMPONENT="$binary" \
                 -t "${REGISTRY}/${binary}:${TAG}" . >/dev/null
    echo "  built ${REGISTRY}/${binary}:${TAG}"
  done
  docker build --quiet -f examples/mcp/go/Dockerfile \
               -t "${REGISTRY}/mcp-hello:${TAG}" . >/dev/null
  echo "  built ${REGISTRY}/mcp-hello:${TAG}"

  note "load images into kind"
  for binary in manager control-plane gateway krypton-proxy mcp-stdio-bridge mcp-hello; do
    kind load docker-image --name "$CLUSTER" "${REGISTRY}/${binary}:${TAG}"
  done

  if [[ "$DEPLOY_LLM" == "true" ]]; then
    note "preload llama.cpp image"
    docker pull "$LLAMA_CPP_IMAGE"
    kind load docker-image --name "$CLUSTER" "$LLAMA_CPP_IMAGE"
  fi
fi

# ---- install ---------------------------------------------------------------

note "helm install"
kubectl create ns "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
helm upgrade --install "$RELEASE" deploy/helm/krypton \
  --namespace "$NAMESPACE" \
  --set image.registry="$REGISTRY" \
  --set image.tag="$TAG" \
  --wait --timeout 5m

note "wait for components"
for c in manager control-plane gateway; do
  kubectl -n "$NAMESPACE" rollout status "deploy/${RELEASE}-${c}" --timeout=3m
done
ok "control plane, gateway and manager are ready"

# ---- port-forwards ---------------------------------------------------------
#
# The Go suite talks over HTTP rather than through kubectl exec, so it reads
# the same way a real client would.

note "port-forward gateway + control plane"
kubectl -n "$NAMESPACE" port-forward "svc/${RELEASE}-gateway" 18080:8080 >/tmp/pf-gw.log 2>&1 &
PF_GW=$!
kubectl -n "$NAMESPACE" port-forward "svc/${RELEASE}-control-plane" 18090:8090 >/tmp/pf-cp.log 2>&1 &
PF_CP=$!
PF_PIDS="$PF_GW $PF_CP"

wait_for_url() {
  local url=$1 name=$2
  for _ in $(seq 1 60); do
    if curl -fsS -o /dev/null "$url" 2>/dev/null; then
      ok "$name reachable at $url"
      return 0
    fi
    sleep 1
  done
  fatal "$name never became reachable at $url"
}
wait_for_url "http://127.0.0.1:18090/healthz" "control plane"
wait_for_url "http://127.0.0.1:18080/healthz" "gateway"

# ---- layer 1: chainsaw -----------------------------------------------------

CHAINSAW=$(command -v chainsaw || echo "$REPO_ROOT/bin/chainsaw")
if [[ ! -x "$CHAINSAW" ]]; then
  echo "! chainsaw not found; skipping the declarative layer"
  echo "  install with: GOBIN=\$PWD/bin go install github.com/kyverno/chainsaw@v0.2.12"
else
  note "chainsaw (declarative reconcile assertions)"
  mkdir -p coverage
  "$CHAINSAW" test test/e2e/chainsaw \
    --config test/e2e/chainsaw/.chainsaw.yaml \
    --report-format JUNIT-TEST \
    --report-path coverage \
    --report-name chainsaw.junit \
    || fatal "chainsaw suite failed"
  ok "chainsaw suite passed"
fi

# ---- layer 2: go behavioural suite -----------------------------------------

note "go e2e (behavioural)"
KRYPTON_E2E_GATEWAY="http://127.0.0.1:18080" \
KRYPTON_E2E_CONTROL_PLANE="http://127.0.0.1:18090" \
KRYPTON_E2E_NAMESPACE="$NAMESPACE" \
KRYPTON_E2E_RELEASE="$RELEASE" \
KRYPTON_E2E_IMAGE="${REGISTRY}/mcp-hello:${TAG}" \
KRYPTON_E2E_LLM="$DEPLOY_LLM" \
  go test -tags e2e -count=1 -timeout 25m -v ./test/e2e/... \
  || fatal "go e2e suite failed"

printf '\n\033[32m✅ e2e passed\033[0m\n'
