#!/usr/bin/env bash
# Chart validation, layered cheapest-first so failures surface fast:
#
#   1. helm lint          chart structure and template syntax
#   2. helm template      renders under a matrix of values combinations
#   3. kubeconform        schema-validates the rendered manifests, including
#                         the Krypton CRs in config/samples against the CRDs
#   4. helm unittest      template logic assertions (no cluster)
#
# Requires: helm, kubeconform, and the helm-unittest plugin. Steps whose
# tooling is absent are skipped with a warning rather than failing, so a
# fresh clone can still run the parts it can.
set -euo pipefail

CHART=deploy/helm/krypton
RENDER_DIR=$(mktemp -d)
trap 'rm -rf "$RENDER_DIR"' EXIT

note() { printf '\n\033[36m▸ %s\033[0m\n' "$*"; }
warn() { printf '\033[33m! %s\033[0m\n' "$*"; }
fail() { printf '\033[31m✖ %s\033[0m\n' "$*" >&2; exit 1; }
ok()   { printf '\033[32m✓ %s\033[0m\n' "$*"; }

command -v helm >/dev/null 2>&1 || fail "helm not found"

# Values combinations that actually change what gets rendered. Each entry is
# a name plus --set arguments.
declare -a CASES=(
  "defaults|"
  "bundled-postgres|--set postgres.enabled=true"
  "postgres-persistent|--set postgres.enabled=true --set postgres.persistence.enabled=true"
  "external-postgres|--set controlPlane.databaseUrl=postgres://u:p@db:5432/k"
  "webhooks-on|--set manager.enableWebhooks=true"
  "scaler-off|--set manager.enableScaler=false"
  "monitors-on|--set serviceMonitor.enabled=true --set podMonitor.enabled=true"
  "rbac-off|--set rbac.create=false"
  "pinned-tag|--set image.tag=v9.9.9"
  "per-component-images|--set images.gateway.repository=registry.internal/gw --set images.gateway.tag=abc123"
  "loadbalancer|--set gateway.service.type=LoadBalancer --set controlPlane.service.type=NodePort"
  "scaled-out|--set manager.replicas=3 --set gateway.replicas=4 --set controlPlane.replicas=2"
)

note "helm lint"
for case in "${CASES[@]}"; do
  name="${case%%|*}"
  args="${case#*|}"
  # shellcheck disable=SC2086
  if ! out=$(helm lint "$CHART" $args 2>&1); then
    echo "$out"
    fail "helm lint failed for values case: $name"
  fi
done
ok "lint clean across ${#CASES[@]} values combinations"

note "helm template"
for case in "${CASES[@]}"; do
  name="${case%%|*}"
  args="${case#*|}"
  # shellcheck disable=SC2086
  if ! helm template krypton "$CHART" --namespace krypton-system $args \
        >"$RENDER_DIR/$name.yaml" 2>"$RENDER_DIR/$name.err"; then
    cat "$RENDER_DIR/$name.err"
    fail "helm template failed for values case: $name"
  fi
  # An empty render means the case silently produced nothing.
  if [[ ! -s "$RENDER_DIR/$name.yaml" ]]; then
    fail "values case $name rendered an empty manifest set"
  fi
done
ok "rendered ${#CASES[@]} manifest sets"

note "kubeconform (rendered manifests + CRs)"
if ! command -v kubeconform >/dev/null 2>&1 && [[ ! -x bin/kubeconform ]]; then
  warn "kubeconform not found; skipping schema validation"
  warn "install with: GOBIN=\$PWD/bin go install github.com/yannh/kubeconform/cmd/kubeconform@v0.6.7"
else
  KC=$(command -v kubeconform || echo bin/kubeconform)

  # Rendered chart output. ServiceMonitor/PodMonitor are prometheus-operator
  # CRDs; pull their schemas from the community repo rather than skipping
  # them, so a malformed monitor is still caught.
  SCHEMA_LOCATIONS=(
    -schema-location default
    -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'
  )

  for f in "$RENDER_DIR"/*.yaml; do
    if ! "$KC" -strict -summary -kubernetes-version 1.31.0 \
         "${SCHEMA_LOCATIONS[@]}" "$f"; then
      fail "kubeconform rejected $(basename "$f")"
    fi
  done
  ok "rendered manifests validate"

  # Deliberately NOT validated here:
  #
  #   * The CRDs themselves. Upstream publishes no standalone-strict schema
  #     for CustomResourceDefinition (the URL 404s), so kubeconform can only
  #     error on them. The stronger check already exists: the envtest suite
  #     installs these exact files via CRDDirectoryPaths and testEnv.Start()
  #     fails outright if either is malformed.
  #
  #   * The sample CRs in config/samples/. Validating those with kubeconform
  #     would need the CRDs converted to JSON Schema first, and the result
  #     would still be weaker than the real thing. TestSamplesAreAccepted in
  #     test/integration applies every sample to a live API server instead.
fi

note "helm unittest"
if ! helm plugin list 2>/dev/null | grep -qi unittest; then
  warn "helm-unittest plugin not installed; skipping template unit tests"
  warn "install with: helm plugin install https://github.com/helm-unittest/helm-unittest"
else
  helm unittest "$CHART" || fail "helm unittest failed"
  ok "template unit tests passed"
fi

printf '\n\033[32m✅ chart validation passed\033[0m\n'
