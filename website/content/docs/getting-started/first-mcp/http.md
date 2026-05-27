---
title: HTTP server
linkTitle: HTTP server
weight: 1
description: Deploy an MCP server that speaks the streamable-HTTP transport.
---

If your MCP server speaks MCP's streamable-HTTP transport natively
(JSON-RPC 2.0 over HTTP POST), Krypton treats it like any other Agent.

> Ports refresher: client traffic to `:8080` (gateway), introspection /
> UI on `:8090` (control plane). See [Ports & endpoints](../first-agent/#ports--endpoints--the-two-minute-mental-model)
> for the full breakdown.

## Container contract

Your container needs to:

- Listen on `spec.port` (default `8080`)
- Accept `POST` at `spec.invocationPath` (default `/`) with JSON-RPC 2.0 bodies
- Implement `initialize`, `notifications/initialized`, `tools/list`, `tools/call`
- Honor `Accept: text/event-stream` if you want strict MCP clients
  (Postman MCP, MCP Inspector) — return SSE frames when requested,
  plain JSON otherwise

## Deploy

```yaml
apiVersion: krypton.ai/v1alpha1
kind: Agent
metadata:
  name: my-mcp
  namespace: agents
spec:
  image: my-registry/my-mcp-server:v1
  imagePullPolicy: IfNotPresent
  protocol: mcp
  port: 8080
  invocationPath: /
  mode: always-on
  minReplicas: 1
  maxReplicas: 3
```

```bash
kubectl apply -f my-mcp.yaml
```

The pod starts, the Service exposes it inside the cluster, the
operator UI surfaces its tools.

## Sample servers

Two pick-your-language flavours, both self-contained:

- [`examples/mcp/go`](https://github.com/kryptonhq/runtime/tree/main/examples/mcp/go)
  — ~200-line Go server, stdlib only
- [`examples/mcp/python`](https://github.com/kryptonhq/runtime/tree/main/examples/mcp/python)
  — minimal [FastMCP](https://github.com/jlowin/fastmcp) server

Both expose three toy tools (`echo`, `add`, `time`/`now`). Deploy
either in the kind cluster:

```bash
kubectl apply -f examples/mcp/go/agent.yaml
# or
kubectl apply -f examples/mcp/python/agent.yaml
```

Then either:

- Open `http://localhost:8090/ui/mcp/agents/mcp-hello` for tool
  introspection in the UI, or
- Hit the typed control-plane endpoints directly:

```bash
curl http://localhost:8090/v1/agents/agents/mcp-hello/mcp/tools
curl -X POST http://localhost:8090/v1/agents/agents/mcp-hello/mcp/tools/add \
     -H 'Content-Type: application/json' \
     -d '{"a": 2, "b": 3}'
```

## Raw MCP protocol path

For tools that want the unmodified MCP wire protocol (MCP Inspector,
Postman's MCP request type, etc.), point them at the **gateway**:

```
POST http://<gateway>/v1/agents/{namespace}/{name}/
Content-Type: application/json
Accept: text/event-stream

{"jsonrpc":"2.0","id":1,"method":"tools/list"}
```

The gateway forwards bodies verbatim and preserves streaming — your
server's `Accept`-aware code path serves SSE if the client asks for it.
