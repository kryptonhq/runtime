#!/usr/bin/env bash
# Minimal end-to-end assertion script. Assumes `hack/local-up.sh` has
# already run and the gateway is reachable on the given URL.
#
# Tests:
#   1. Cold start: first invocation against an idle (scale-to-zero) agent
#      returns 200 within the startup timeout
#   2. Warm: subsequent invocation responds with low latency
#   3. /v1/agents shows the agent in Phase=Ready after cold start

set -euo pipefail

GATEWAY_URL=${GATEWAY_URL:-http://localhost:8080}
CP_URL=${CP_URL:-http://localhost:8090}
NAMESPACE=${NAMESPACE:-agents}
AGENT=${AGENT:-mcp-hello}

note() { printf '\n\033[36m▸ %s\033[0m\n' "$*"; }
fail() { printf '\033[31m✖ %s\033[0m\n' "$*" >&2; exit 1; }
# BSD date (macOS) lacks %N for sub-second precision; use python3 instead.
now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

note "invocation 1 (cold)"
start=$(now_ms)
body=$(curl -fsS -X POST "${GATEWAY_URL}/v1/agents/${NAMESPACE}/${AGENT}/" \
            -H 'Content-Type: application/json' \
            -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}') || fail "cold invocation failed"
end=$(now_ms)
echo "$body" | grep -q '"tools"' || fail "unexpected response: $body"
echo "  ${body}"
echo "  latency: $((end - start))ms"

note "invocation 2 (warm)"
start=$(now_ms)
curl -fsS -X POST "${GATEWAY_URL}/v1/agents/${NAMESPACE}/${AGENT}/" \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' >/dev/null || fail "warm invocation failed"
end=$(now_ms)
warm_ms=$((end - start))
echo "  latency: ${warm_ms}ms"
if [[ $warm_ms -gt 3000 ]]; then
  fail "warm invocation too slow (${warm_ms}ms); not actually warm?"
fi

note "control plane sees the agent as Ready"
status=$(curl -fsS "${CP_URL}/v1/agents/${NAMESPACE}/${AGENT}/status" || fail "GET status failed")
echo "  $status"
echo "$status" | grep -q '"phase":"Ready"' || fail "agent phase != Ready"

printf '\n\033[32m✅ smoke test passed\033[0m\n'
