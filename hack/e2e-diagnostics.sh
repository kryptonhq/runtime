#!/usr/bin/env bash
# Dumps everything needed to debug a failed e2e run into a directory the CI
# job uploads as an artifact. Debugging a red e2e without pod logs and events
# is guesswork, so this runs unconditionally on failure.
#
# Never fails: a diagnostic collector that errors out defeats its purpose.
set -uo pipefail

OUT=${OUT:-/tmp/krypton-e2e-diagnostics}
NAMESPACE=${NAMESPACE:-krypton-system}
AGENT_NAMESPACES=${AGENT_NAMESPACES:-agents models}

mkdir -p "$OUT"

say() { printf '\033[36m  · %s\033[0m\n' "$*"; }

say "cluster-wide overview"
kubectl get nodes -o wide            >"$OUT/nodes.txt"              2>&1
kubectl get all --all-namespaces -o wide >"$OUT/all-resources.txt"  2>&1
kubectl get events --all-namespaces --sort-by=.lastTimestamp \
                                     >"$OUT/events.txt"             2>&1

say "krypton CRs"
kubectl get agents,models --all-namespaces -o yaml >"$OUT/krypton-crs.yaml" 2>&1
kubectl get crd -o name | grep krypton >"$OUT/crds.txt" 2>&1

say "helm release"
helm list -n "$NAMESPACE" >"$OUT/helm-list.txt" 2>&1
helm get values krypton -n "$NAMESPACE" >"$OUT/helm-values.yaml" 2>&1

# Logs for every pod in every namespace we care about, including previous
# containers — a CrashLoopBackOff's useful output is in the previous one.
for ns in "$NAMESPACE" $AGENT_NAMESPACES; do
  pods=$(kubectl -n "$ns" get pods -o name 2>/dev/null) || continue
  [[ -z "$pods" ]] && continue
  say "logs for namespace $ns"
  mkdir -p "$OUT/logs/$ns"
  while read -r pod; do
    [[ -z "$pod" ]] && continue
    name=${pod#pod/}
    kubectl -n "$ns" describe "$pod" >"$OUT/logs/$ns/${name}.describe.txt" 2>&1
    containers=$(kubectl -n "$ns" get "$pod" \
      -o jsonpath='{.spec.containers[*].name}' 2>/dev/null)
    for c in $containers; do
      kubectl -n "$ns" logs "$pod" -c "$c" --tail=2000 \
        >"$OUT/logs/$ns/${name}.${c}.log" 2>&1
      kubectl -n "$ns" logs "$pod" -c "$c" --previous --tail=2000 \
        >"$OUT/logs/$ns/${name}.${c}.previous.log" 2>&1
      # An empty "previous" file is noise; drop it.
      [[ -s "$OUT/logs/$ns/${name}.${c}.previous.log" ]] || \
        rm -f "$OUT/logs/$ns/${name}.${c}.previous.log"
    done
  done <<<"$pods"
done

say "port-forward logs"
cp /tmp/pf-gw.log "$OUT/port-forward-gateway.log" 2>/dev/null || true
cp /tmp/pf-cp.log "$OUT/port-forward-control-plane.log" 2>/dev/null || true

printf '\033[36m  diagnostics written to %s\033[0m\n' "$OUT"
exit 0
