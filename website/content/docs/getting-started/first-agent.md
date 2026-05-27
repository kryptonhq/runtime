---
title: Deploying your first Agent
linkTitle: First Agent
weight: 2
description: Deploy a custom container as a Krypton Agent.
---

This walks through deploying a custom agent into a running Krypton
cluster. Krypton treats your container as a black box that speaks A2A
(or MCP / plain HTTP).

## Ports & endpoints — the two-minute mental model

Krypton exposes **two** HTTP services. Talk to the right one:

| Port | Service | What lives here |
| ---: | ------- | --------------- |
| **8080** | **Gateway** | All agent traffic. Anything under `/v1/agents/{ns}/{name}/*` is reverse-proxied to your pod — the protocol RPC at `/`, the A2A card at `/.well-known/agent-card.json`, OAuth callbacks at `/oauth/...`, MCP SSE streams, anything. The gateway strips the `/v1/agents/{ns}/{name}` prefix; your container sees the original sub-path. |
| **8090** | **Control plane** | Web UI and introspection APIs (`/v1/agents`, `/v1/agents/{ns}/{name}/mcp/tools`, `/v1/agents/{ns}/{name}/status`). Operator tooling only — **never** the path your clients use to invoke an agent. |

Rule of thumb: if it's a normal A2A / MCP / HTTP client, point it at
`:8080`. If it's a browser or `kubectl`-adjacent tool, `:8090`.

## 1. Pick a starting point

Four ready-to-build A2A agents live under [`examples/agent`](https://github.com/kryptonhq/runtime/tree/main/examples/agent).
Start with the no-LLM one to confirm your install works, then graduate
to a framework example when you're ready to plug in your own logic.

| Path | LLM? | Framework | Source |
| ---- | :-: | --------- | ------ |
| [`examples/agent/python/helloworld`](https://github.com/kryptonhq/runtime/tree/main/examples/agent/python/helloworld) | — | Bare `a2a-sdk` | [a2a-samples/helloworld](https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/helloworld) |
| [`examples/agent/go`](https://github.com/kryptonhq/runtime/tree/main/examples/agent/go) | Gemini | Google ADK-Go | [adk.dev quickstart](https://adk.dev/a2a/quickstart-exposing-go/) |
| [`examples/agent/python/adk`](https://github.com/kryptonhq/runtime/tree/main/examples/agent/python/adk) | Gemini | Google ADK (Python) | [a2a-samples/adk_facts](https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/adk_facts) |
| [`examples/agent/python/langgraph`](https://github.com/kryptonhq/runtime/tree/main/examples/agent/python/langgraph) | Gemini | LangGraph | [a2a-samples/langgraph](https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/langgraph) |

The `helloworld` agent needs **no API key and no Secret** — it just
echoes whatever the client sends.

The container contract is the same for all of them:

- Listen on an HTTP port (`spec.port`, defaults to `8080`)
- Serve A2A JSON-RPC at `spec.invocationPath` (defaults to `/`)
- Expose the [agent card](https://a2a-protocol.org/) at
  `/.well-known/agent-card.json`

## 2. Build and load

Each example has its own `Dockerfile` and `agent.yaml`. The ADK Python
flavour, for instance:

```bash
docker build -f examples/agent/python/adk/Dockerfile \
  -t krypton/adk-agent:dev examples/agent/python/adk
kind load docker-image --name krypton-dev krypton/adk-agent:dev
```

The two LLM-backed samples (`adk`, `langgraph`) need a `GOOGLE_API_KEY`
Secret in the `agents` namespace — see each example's README for the
exact `kubectl create secret` call.

## 3. Apply the Agent CR

```bash
kubectl apply -f examples/agent/python/adk/agent.yaml
```

The shipped `agent.yaml` files look like this (ADK Python shown):

```yaml
apiVersion: krypton.ai/v1alpha1
kind: Agent
metadata:
  name: adk
  namespace: agents
spec:
  image: krypton/adk-agent:dev
  imagePullPolicy: IfNotPresent
  runtime: python
  framework: google-adk
  protocol: a2a
  mode: always-on
  minReplicas: 1
  maxReplicas: 3
  concurrency: 4
  port: 8080
  invocationPath: /
  env:
    - name: GOOGLE_API_KEY
      valueFrom:
        secretKeyRef: { name: adk-secrets, key: GOOGLE_API_KEY }
```

## 4. Invoke

A2A agents expose their card at `/.well-known/agent-card.json` and accept
JSON-RPC `message/send` calls at the invocation path. Through the gateway:

```bash
# Discover the agent card
curl http://localhost:8080/v1/agents/agents/adk/.well-known/agent-card.json

# Send a message
curl -X POST http://localhost:8080/v1/agents/agents/adk/ \
     -H 'Content-Type: application/json' \
     -d '{
       "jsonrpc": "2.0",
       "id": "1",
       "method": "message/send",
       "params":{"message":{"messageId":"1","role":"user",
           "parts": [{"kind": "text", "text": "Tell me a fun fact about octopuses."}]
         }
       }
     }'
```

Always-on agents keep at least `minReplicas` pods warm — every call is
served immediately by an existing pod.

## Routing under the hood

```
client → gateway → krypton-proxy sidecar → your container
                            │
                            └──► /_krypton/inflight
                                 (read by the scaler in the manager)
```

The sidecar enforces `spec.concurrency` per pod; over the cap returns
503 + `Retry-After`. The scaler in the manager observes inflight per pod,
computes `ceil(inflight / concurrency)`, and writes
`status.desiredReplicas` — the reconciler scales the Deployment to match.

## What's next

- [Agent CRD reference](../../reference/agent-crd/) — every spec field
- [Metrics](/docs/observability/metrics/) — what the runtime exposes
- [Components](../../architecture/components/) — what's running where
