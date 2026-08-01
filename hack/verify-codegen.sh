#!/usr/bin/env bash
# Drift gates for artifacts that are committed but derived.
#
# 1. CRDs are currently hand-written (see the
#    `controller-gen.kubebuilder.io/version: hand-written-bootstrap`
#    annotation), so `make manifests` is NOT run here — it would clobber
#    them. Instead we assert that every field in the Go types has a
#    corresponding property in the CRD schema, which is the failure mode
#    hand-maintenance actually produces. The envtest round-trip test in
#    test/integration is the belt to this braces.
# 2. zz_generated.deepcopy.go IS controller-gen output, so it must match.
# 3. The chart ships its own copy of each CRD because Helm does not
#    template crds/. The two trees must stay byte-identical.
set -euo pipefail

note() { printf '\n\033[36m▸ %s\033[0m\n' "$*"; }
fail() { printf '\033[31m✖ %s\033[0m\n' "$*" >&2; exit 1; }
ok()   { printf '\033[32m✓ %s\033[0m\n' "$*"; }

note "deepcopy is up to date"
make generate >/dev/null
if ! git diff --quiet -- api/; then
  git --no-pager diff --stat -- api/
  fail "zz_generated.deepcopy.go is stale — run 'make generate' and commit the result"
fi
ok "deepcopy matches the Go types"

note "chart CRD copies match config/crd/bases"
# config/crd/bases/krypton.ai_agents.yaml  <->  deploy/helm/krypton/crds/agents.krypton.ai.yaml
declare -A CRD_PAIRS=(
  ["config/crd/bases/krypton.ai_agents.yaml"]="deploy/helm/krypton/crds/agents.krypton.ai.yaml"
  ["config/crd/bases/krypton.ai_models.yaml"]="deploy/helm/krypton/crds/models.krypton.ai.yaml"
)
for src in "${!CRD_PAIRS[@]}"; do
  dst="${CRD_PAIRS[$src]}"
  [[ -f "$src" ]] || fail "missing $src"
  [[ -f "$dst" ]] || fail "missing $dst"
  if ! diff -u "$src" "$dst"; then
    fail "$dst has drifted from $src — the chart ships a second copy; keep them identical"
  fi
done
ok "chart CRD copies are identical"

note "every AgentSpec/ModelSpec field exists in the CRD schema"
# Compares json tags on the Go spec struct against the CRD's
# spec.properties keys. A field present in Go but absent from the schema
# gets silently pruned by the API server at write time.
check_fields() {
  local go_file=$1 go_type=$2 crd_file=$3
  local go_fields crd_fields missing

  go_fields=$(awk "/^type ${go_type} struct/,/^}/" "$go_file" |
    grep -oE 'json:"[a-zA-Z]+' | sed 's/json:"//' | sort -u)

  # Grab the properties block nested under spec: in the CRD schema. The
  # hand-written CRDs indent spec properties by 16 spaces.
  crd_fields=$(awk '/^            spec:/,/^            status:/' "$crd_file" |
    grep -oE '^                [a-zA-Z]+:' | tr -d ' :' | sort -u)

  missing=$(comm -23 <(echo "$go_fields") <(echo "$crd_fields"))
  if [[ -n "$missing" ]]; then
    echo "fields in ${go_type} with no CRD property:" >&2
    echo "$missing" | sed 's/^/  - /' >&2
    fail "$crd_file is missing schema for the fields above; the API server will prune them"
  fi
  ok "${go_type}: $(echo "$go_fields" | wc -l | tr -d ' ') fields all present in $(basename "$crd_file")"
}

check_fields api/v1alpha1/agent_types.go AgentSpec config/crd/bases/krypton.ai_agents.yaml
check_fields api/v1alpha1/model_types.go ModelSpec config/crd/bases/krypton.ai_models.yaml

printf '\n\033[32m✅ codegen and CRD drift gates passed\033[0m\n'
