# Krypton

Kubernetes-native runtime for AI agents and MCP servers. Deploy any
A2A / MCP / HTTP container as a first-class `Agent` CRD; Krypton
handles scheduling, scaling, invocation routing, and lifecycle.

## Why Krypton

- **Framework-agnostic.** LangGraph, Google ADK, custom HTTP, plain
  MCP — if it speaks HTTP or stdio, it runs.
- **Protocol-native.** First-class A2A and MCP routing through the
  gateway; no shim, no envelope rewriting.
- **Your container, as-is.** No SDK to import, no base image to
  inherit. Bring a Dockerfile, point an `Agent` resource at it.
- **Batteries included.** Built-in operator UI, Prometheus metrics,
  scale-from-zero activator, per-pod proxy sidecar.
- **One CRD describes the whole thing.** Image, protocol, scaling,
  routing, env — all in a single `Agent` resource.

## Quickstart

Install the chart:

```bash
helm install krypton oci://ghcr.io/kryptonhq/charts/krypton \
  --namespace krypton-system \
  --create-namespace \
  --set image.tag=v0.1.0
```

Deploy the no-secrets helloworld agent and invoke it:

```bash
kubectl apply -f https://raw.githubusercontent.com/kryptonhq/runtime/main/examples/agent/python/helloworld/agent.yaml
kubectl -n krypton-system port-forward svc/krypton-gateway 8080:8080 &

curl -X POST http://localhost:8080/v1/agents/agents/helloworld/ \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":"1","method":"message/send",
          "params":{"message":{"messageId":"m1","role":"user",
            "parts":[{"kind":"text","text":"ping"}]}}}'
```

You should see a JSON-RPC reply with `"state": "completed"`. The
agent is also visible in the operator UI on the control plane
(`kubectl -n krypton-system port-forward svc/krypton-control-plane 8090:8090`,
then open <http://localhost:8090/ui/>).

## Where next

| You want to...                          | Go to                                                                 |
| --------------------------------------- | --------------------------------------------------------------------- |
| Read the full docs                      | <https://docs.krypton.ai>                                             |
| Browse runnable examples                | [`examples/`](./examples)                                             |
| Tune the Helm chart                     | [`deploy/helm/krypton/`](./deploy/helm/krypton)                       |
| File an issue or contribute             | <https://github.com/kryptonhq/runtime/issues>                         |

## Status

> **Pre-alpha.** APIs (CRD shape, gateway URL scheme, Helm values) are
> still moving. Pin to a tagged release if you're trying it out;
> expect breaking changes between minor versions until `v1.0`.

## License

Apache 2.0 — see [LICENSE](./LICENSE).
