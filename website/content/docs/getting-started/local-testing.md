---
title: Local testing with kind
linkTitle: Local testing (kind)
weight: 6
description: One-shot kind cluster + sample agent for laptop testing.
---

End-to-end loop for running Krypton on your laptop. Tested on macOS
(OrbStack and Docker Desktop) and Linux.

## Prerequisites

- Docker (or OrbStack)
- [kind](https://kind.sigs.k8s.io/) ≥ 0.24 — `brew install kind`
- [Helm](https://helm.sh) ≥ 3.14 — `brew install helm`
- [kubectl](https://kubernetes.io/docs/tasks/tools/) — `brew install kubectl`
- Node 22 + pnpm 10 (for the UI build)
- Go 1.25 (only required for unit tests)

One-liner on macOS:

```bash
brew install kind helm kubectl
```

## One-shot install

```bash
make deploy-dev
```

What that runs ([`hack/local-up.sh`](https://github.com/kryptonhq/runtime/blob/main/hack/local-up.sh)):

1. Creates a `krypton-dev` kind cluster (or reuses one)
2. Builds the React UI and stages it for `go:embed`
3. Builds five Docker images: `manager`, `control-plane`, `gateway`,
   `krypton-proxy`, `mcp-hello` — all tagged `krypton/<name>:dev`
4. Loads them into the kind cluster
5. `helm upgrade --install`s the chart from `deploy/helm/krypton`
6. Applies the [mcp-hello smoke-test agent](https://github.com/kryptonhq/runtime/blob/main/examples/mcp/go/agent.yaml)

Idempotent — re-run any time after editing code.

## LLM local test

To test model serving in the same local loop, run:

```bash
make deploy-dev-llm
```

That runs the normal dev setup, preloads the `llama.cpp` server image
into kind, and applies the Qwen2.5 0.5B `Model` sample from
`config/samples/llm/qwen2.5-0.5b.yaml`.

The model pod still downloads the GGUF weights from Hugging Face on
first start, so this path needs network access and can take a few
minutes.

After the script prints the port-forward command, wait for the model
Deployment and call it:

```bash
kubectl -n krypton-system port-forward svc/krypton-gateway 8080:8080 &
kubectl -n models rollout status deploy/qwen2-0-5b --timeout=15m

curl -s http://localhost:8080/v1/models | jq

curl -s http://localhost:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "qwen2-0-5b",
    "messages": [{"role": "user", "content": "Say hi in one word."}]
  }' | jq
```

You can use the script directly when you want to override the sample:

```bash
DEPLOY_LLM=true \
LLM_SAMPLE=config/samples/llm/tinyllama-1.1b.yaml \
LLM_NAME=tinyllama-1-1b \
./hack/local-up.sh
```

## Manual usage after install

```bash
# Forward the gateway and control plane in two terminals.
kubectl -n krypton-system port-forward svc/krypton-gateway 8080:8080 &
kubectl -n krypton-system port-forward svc/krypton-control-plane 8090:8090 &

# Open the UI.
open http://localhost:8090/ui/

# Invoke the mcp-hello agent — first call may take a few seconds while
# the pod becomes ready; subsequent calls are sub-millisecond.
curl -X POST http://localhost:8080/v1/agents/agents/mcp-hello/ \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Expected response:

```json
{
  "delay": "100ms",
  "echo": "{\"hello\":\"world\"}",
  "method": "POST",
  "path": "/",
  "servedBy": "echo-xxxxxxxx-yyyyy"
}
```

## Smoke test

After port-forwards are running:

```bash
make e2e-local
```

This asserts:

1. A cold invocation returns 200 with the expected echo body
2. A second invocation is warm (sub-3s)
3. The control plane reports `phase=Ready`

## Watching things happen

```bash
kubectl get pods -A -w | grep -E 'agents|krypton'
kubectl get agents -A -w
```

The echo agent runs in always-on mode with `minReplicas: 1`, so one pod
stays up for the lifetime of the CR. The scaler bumps replicas up under
load (per `spec.concurrency`) and back down to `minReplicas` when load
drops.

## Teardown

```bash
kind delete cluster --name krypton-dev
```

## Troubleshooting

See [Troubleshooting & FAQ](/docs/troubleshooting/).
