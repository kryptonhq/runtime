# mcp-hello-py

A tiny [FastMCP][fastmcp] server — the Python twin of
[examples/mcp/go](../go). Three toy tools:

| Tool   | Input              | Output                       |
| ------ | ------------------ | ---------------------------- |
| `echo` | `{"message":"..."}` | The message echoed back.     |
| `add`  | `{"a":1,"b":2}`    | The sum as a string.         |
| `now`  | `{}`               | Current UTC time (RFC 3339). |

Speaks MCP over streamable HTTP — JSON-RPC 2.0 at `POST /`. No LLM.

## Build

```bash
docker build -f examples/mcp/python/Dockerfile \
  -t krypton/mcp-hello-py:dev examples/mcp/python
kind load docker-image --name krypton-dev krypton/mcp-hello-py:dev
```

## Deploy

```bash
kubectl apply -f examples/mcp/python/agent.yaml
```

## Invoke

```bash
# List tools
curl -X POST http://localhost:8080/v1/agents/agents/mcp-hello-py/ \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# Call a tool
curl -X POST http://localhost:8080/v1/agents/agents/mcp-hello-py/ \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":3}}}'
```

Or via the control plane's typed endpoints:

```bash
curl http://localhost:8090/v1/agents/agents/mcp-hello-py/mcp/tools
curl -X POST http://localhost:8090/v1/agents/agents/mcp-hello-py/mcp/tools/add \
     -H 'Content-Type: application/json' \
     -d '{"a": 2, "b": 3}'
```

[fastmcp]: https://github.com/jlowin/fastmcp
