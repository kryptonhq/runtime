<p align="center">
  <img src="website/static/brand/mark-light.svg" alt="Krypton logo" width="96" height="96">
</p>

<h1 align="center">Krypton Runtime</h1>

<p align="center">
  Kubernetes-native serving for AI agents, self-hosted LLMs, and MCP servers.
</p>

<p align="center">
  <a href="https://github.com/kryptonhq/runtime/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/kryptonhq/runtime/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/kryptonhq/runtime/releases"><img alt="Release" src="https://img.shields.io/github/v/release/kryptonhq/runtime?include_prereleases&sort=semver"></a>
  <a href="https://www.kryptonhq.com/docs/"><img alt="Docs" src="https://img.shields.io/badge/docs-kryptonhq.com-6366f1"></a>
  <a href="go.mod"><img alt="Go" src="https://img.shields.io/badge/go-1.25-00ADD8"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-blue"></a>
</p>

Krypton lets platform teams run AI workloads as ordinary Kubernetes
resources. Declare an `Agent` or `Model`, let the controller create the
workload, and send traffic through stable HTTP, A2A, MCP, or
OpenAI-compatible endpoints.

## What it does

| Capability | What you get |
| ---------- | ------------ |
| **Agents** | Run A2A, MCP, or plain HTTP containers with an `Agent` CRD. Bring LangGraph, Google ADK, custom services, or your own image. |
| **Self-hosted LLMs** | Run Hugging Face GGUF models with llama.cpp using a `Model` CRD and OpenAI-compatible `/v1/models` and `/v1/chat/completions` routes. |
| **MCP servers** | Host native HTTP MCP servers, or wrap stdio MCP binaries with the bundled `mcp-stdio-bridge`. |
| **Gateway routing** | One public gateway handles agent invocation, model routing, streaming responses, and protocol-specific paths. |
| **Operator UI** | A lightweight control-plane UI lists agents, models, status, and MCP tools. |
| **Scaling signals** | Per-pod sidecars enforce concurrency, expose in-flight counts, and feed the scaler. |
| **Observability** | Prometheus metrics for gateway traffic, model calls, sidecar load, scaler decisions, and control-plane health. |
| **Helm-first install** | Install the runtime as a single OCI Helm chart and keep ingress, TLS, auth, and rate limiting in your existing platform layer. |

## Quickstart

Install Krypton:

```bash
helm install krypton oci://ghcr.io/kryptonhq/charts/krypton \
  --namespace krypton-system \
  --create-namespace
```

Deploy the no-secrets helloworld agent:

```bash
kubectl apply -f https://raw.githubusercontent.com/kryptonhq/runtime/main/examples/agent/python/helloworld/agent.yaml
kubectl -n krypton-system port-forward svc/krypton-gateway 8080:8080 &
```

Invoke it through the gateway:

```bash
curl -X POST http://localhost:8080/v1/agents/agents/helloworld/ \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"1","method":"message/send",
       "params":{"message":{"messageId":"m1","role":"user",
       "parts":[{"kind":"text","text":"ping"}]}}}'
```

Open the operator UI:

```bash
kubectl -n krypton-system port-forward svc/krypton-control-plane 8090:8090
open http://localhost:8090/ui/
```

## Try an LLM

```bash
kubectl create ns models --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f https://raw.githubusercontent.com/kryptonhq/runtime/main/config/samples/llm/qwen2.5-0.5b.yaml

curl -s http://localhost:8080/v1/models | jq
```

Any OpenAI SDK can use `http://localhost:8080/v1` as its `base_url`.

## Local development

For a full kind-based loop:

```bash
make deploy-dev
```

For model serving in the same loop:

```bash
make deploy-dev-llm
```

## Learn more

| Resource | Link |
| -------- | ---- |
| Documentation | <https://www.kryptonhq.com/docs/> |
| Examples | [`examples/`](./examples) |
| Helm chart | [`deploy/helm/krypton/`](./deploy/helm/krypton) |
| CRD reference | [`api/v1alpha1/`](./api/v1alpha1) |
| Issues | <https://github.com/kryptonhq/runtime/issues> |

## Status

Krypton is pre-alpha. CRDs, Helm values, gateway paths, and image names
may change before `v1.0`. Pin tagged releases for experiments and expect
breaking changes between early versions.

## License

Apache 2.0. See [LICENSE](./LICENSE).
