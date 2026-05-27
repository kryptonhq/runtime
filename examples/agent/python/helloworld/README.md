# Hello-World A2A on Krypton (no LLM, no secrets)

A minimal A2A agent that echoes whatever the client sends — no LLM, no
API keys, no external calls. Use this as the "does my Krypton install
actually serve A2A?" smoke test, the same way [`examples/mcp/go`](../../../mcp/go)
serves as the MCP smoke test.

The interesting bit ([`agent_executor.py`](./agent_executor.py)) is
copied verbatim from [a2a-samples/helloworld][upstream]. We replaced
the upstream `__main__.py` because it hard-codes
`uvicorn.run(host='127.0.0.1', port=9999)` — fine for a laptop, broken
inside a container. Our [`__main__.py`](./__main__.py) builds the same
Starlette app but binds to `0.0.0.0:$PORT` (default `8080`).

## Build

```bash
docker build -f examples/agent/python/helloworld/Dockerfile \
  -t krypton/helloworld-agent:dev examples/agent/python/helloworld
kind load docker-image --name krypton-dev krypton/helloworld-agent:dev
```

## Deploy

```bash
kubectl apply -f examples/agent/python/helloworld/agent.yaml
```

No Secret needed.

## Invoke

```bash
# Agent card
curl http://localhost:8080/v1/agents/agents/helloworld/.well-known/agent-card.json

# Send a message
curl -X POST http://localhost:8080/v1/agents/agents/helloworld/ \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":"1","method":"message/send",
          "params":{"message":{"messageId":"1","role":"user",
            "parts":[{"kind":"text","text":"ping"}]}}}'
```

Expected response artifact: `Hello, World! I have received your request (ping)`.

## Attribution

[`agent_executor.py`](./agent_executor.py),
[`__init__.py`](./__init__.py), [`pyproject.toml`](./pyproject.toml),
[`uv.lock`](./uv.lock), and [`UPSTREAM_README.md`](./UPSTREAM_README.md)
are copied unmodified from
<https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/helloworld>
(Apache 2.0). [`__main__.py`](./__main__.py),
[`requirements.txt`](./requirements.txt),
[`Dockerfile`](./Dockerfile), and [`agent.yaml`](./agent.yaml) are
Krypton-specific.

[upstream]: https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/helloworld
