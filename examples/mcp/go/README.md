# mcp-hello

A tiny Model Context Protocol server used to smoke-test Krypton's MCP
support. Three toy tools:

| Tool   | Input                       | Output                            |
| ------ | --------------------------- | --------------------------------- |
| `echo` | `{"message": "..."}`        | The message echoed back.          |
| `add`  | `{"a": 1, "b": 2}`          | The sum as a string.              |
| `time` | `{}`                        | Current UTC time (RFC 3339).      |

Speaks MCP over streamable HTTP — JSON-RPC 2.0 at `POST /`. No external
deps, no LLM calls.

## Build

```bash
docker build -f examples/mcp/go/Dockerfile -t krypton/mcp-hello:dev .
kind load docker-image --name krypton-dev krypton/mcp-hello:dev
```

## Deploy

```bash
kubectl apply -f examples/mcp/go/agent.yaml
```

## Invoke

Through the gateway:

```bash
# List tools
curl -X POST http://localhost:8080/v1/agents/agents/mcp-hello/ \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# Call a tool
curl -X POST http://localhost:8080/v1/agents/agents/mcp-hello/ \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":3}}}'
```

Or via the control plane's typed endpoints (introspection-friendly):

```bash
# Discovered tools as a JSON list
curl http://localhost:8090/v1/agents/agents/mcp-hello/mcp/tools

# Call a tool — body = arguments object
curl -X POST http://localhost:8090/v1/agents/agents/mcp-hello/mcp/tools/add \
     -H 'Content-Type: application/json' \
     -d '{"a": 2, "b": 3}'
```

The control plane handles the MCP handshake transparently and unwraps
the `content` envelope.
