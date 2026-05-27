#!/usr/bin/env bash
# Local end-to-end loop: kind cluster → build images → load → helm install
# → deploy the mcp-hello smoke-test agent → invoke through the gateway.
#
# Idempotent; safe to re-run. Tear everything down with `make kind-down`.

set -euo pipefail

CLUSTER=${CLUSTER:-krypton-dev}
NAMESPACE=${NAMESPACE:-krypton-system}
RELEASE=${RELEASE:-krypton}
TAG=${TAG:-dev}
REGISTRY=${REGISTRY:-krypton}

note()  { printf '\n\033[36m▸ %s\033[0m\n' "$*"; }
fatal() { printf '\033[31m✖ %s\033[0m\n' "$*" >&2; exit 1; }

require() { command -v "$1" >/dev/null 2>&1 || fatal "missing required tool: $1"; }

require kind
require docker
require helm
require kubectl
require pnpm

note "kind cluster"
if ! kind get clusters | grep -qx "$CLUSTER"; then
  kind create cluster --name "$CLUSTER" --config hack/kind.yaml
fi
kubectl config use-context "kind-${CLUSTER}" >/dev/null

note "build UI"
( cd ui && pnpm install --frozen-lockfile && pnpm build )
rm -rf internal/controlplane/embed/dist
mkdir -p internal/controlplane/embed/dist
cp -R ui/dist/. internal/controlplane/embed/dist/
touch internal/controlplane/embed/dist/.gitkeep

note "build images"
for binary in manager control-plane gateway krypton-proxy mcp-stdio-bridge; do
  docker build --build-arg COMPONENT="$binary" \
               -t "${REGISTRY}/${binary}:${TAG}" .
done
docker build -f examples/mcp/go/Dockerfile -t "${REGISTRY}/mcp-hello:${TAG}" .

note "load images into kind"
for binary in manager control-plane gateway krypton-proxy mcp-stdio-bridge mcp-hello; do
  kind load docker-image --name "$CLUSTER" "${REGISTRY}/${binary}:${TAG}"
done

note "helm install"
kubectl create ns "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
helm upgrade --install "$RELEASE" deploy/helm/krypton \
  --namespace "$NAMESPACE" \
  --set image.registry="$REGISTRY" \
  --set image.tag="$TAG"

note "wait for components"
kubectl -n "$NAMESPACE" rollout status deploy/${RELEASE}-manager
kubectl -n "$NAMESPACE" rollout status deploy/${RELEASE}-control-plane
kubectl -n "$NAMESPACE" rollout status deploy/${RELEASE}-gateway

note "deploy the mcp-hello smoke-test agent"
kubectl apply -f examples/mcp/go/agent.yaml

note "wait for mcp-hello to be reconciled (Deployment created)"
for _ in $(seq 1 30); do
  if kubectl -n agents get deploy mcp-hello >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
kubectl -n agents get deploy mcp-hello || fatal "mcp-hello Deployment not created"

cat <<EOF

✅ Krypton is up.

Forward the gateway and invoke the mcp-hello agent:

    kubectl -n ${NAMESPACE} port-forward svc/${RELEASE}-gateway 8080:8080 &
    curl -X POST http://localhost:8080/v1/agents/agents/mcp-hello/ \\
         -H 'Content-Type: application/json' \\
         -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

Open the UI:

    kubectl -n ${NAMESPACE} port-forward svc/${RELEASE}-control-plane 8090:8090 &
    open http://localhost:8090/ui/

Teardown:

    kind delete cluster --name ${CLUSTER}
EOF
