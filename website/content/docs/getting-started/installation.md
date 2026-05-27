---
title: Installation
weight: 1
description: Install Krypton with Helm and deploy a no-secrets agent in two commands.
---

Krypton ships as a single Helm chart with four components: **manager**
(controller + scaler), **control plane** (REST API + UI), **gateway**
(public ingress + activator), and a **per-pod proxy sidecar** the
manager injects into agent pods.

If you just want to kick the tyres on a laptop, jump to
[Local testing](../local-testing/) — that wraps everything below in
`make deploy-dev`.

## 1. Install the chart

```bash
helm install krypton oci://ghcr.io/kryptonhq/charts/krypton \
  --namespace krypton-system \
  --create-namespace \
  --set controlPlane.databaseUrl="postgres://user:pass@host:5432/db"
```

This installs the most recent chart; its `appVersion` pins the
matching image tags. To pin a specific release, pass
`--version {{< version-bare >}}` (latest stable is **{{< version >}}**).

`controlPlane.databaseUrl` points at a managed Postgres for the
agent-registry mirror. Leave empty to use an in-memory store
(acceptable for dev or single-replica installs).

For production, you'll front the `krypton-gateway` Service with an L7
ingress (Gateway API, Nginx, etc.) — covered in
[Bring your own ingress](../ingress/) once you've got an agent running.

## 2. Open the UI and confirm health

The control plane serves a small operator UI:

```bash
kubectl -n krypton-system port-forward svc/krypton-control-plane 8090:8090
open http://localhost:8090/ui/
```

You should land on an **Agents** list (empty) and an **MCP servers**
list (empty). If the page renders and both lists load without errors,
the manager, control plane, and Postgres mirror are all healthy.

> Ports refresher: `:8090` is operator tooling only. Agent traffic goes
> through the gateway on `:8080` — see
> [Ports & endpoints](../first-agent/#ports--endpoints--the-two-minute-mental-model).

## 3. Deploy the helloworld agent (two commands)

`examples/agent/python/helloworld` is a no-LLM, no-secrets A2A agent
that just echoes — perfect for confirming the end-to-end path works:

```bash
kubectl apply -f https://raw.githubusercontent.com/kryptonhq/runtime/main/examples/agent/python/helloworld/agent.yaml
kubectl -n krypton-system port-forward svc/krypton-gateway 8080:8080 &
```

Then invoke it:

```bash
curl -X POST http://localhost:8080/v1/agents/agents/helloworld/ \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":"1","method":"message/send",
          "params":{"message":{"messageId":"m1","role":"user",
            "parts":[{"kind":"text","text":"ping"}]}}}'
```

You'll get back:

```json
{ "result": { "artifacts": [{ "parts": [{ "kind": "text",
   "text": "Hello, World! I have received your request (ping)" }]}],
   "status": { "state": "completed" } } }
```

The agent is also visible in the UI now, with `Phase: Ready` and the
Invoke widget pre-populated against the same gateway endpoint.

## Where to go next

- **Build and deploy your own agent** (LangGraph, ADK, anything) → [Deploying your first Agent](../first-agent/)
- **Deploy an MCP server** → [Deploying your first MCP](../first-mcp/)
- **Production-grade ingress, TLS, rate limiting** → [Bring your own ingress](../ingress/)
- **End-to-end local loop with kind** → [Local testing](../local-testing/)
