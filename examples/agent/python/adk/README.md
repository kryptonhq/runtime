# Google ADK + A2A on Krypton

A "fun facts" agent built with the [Agent Development Kit (ADK)][adk] and
exposed via [A2A][a2a]. Copied verbatim from the
[`adk_facts` sample][upstream] — see [UPSTREAM_README.md](./UPSTREAM_README.md)
for the original walkthrough.

The upstream code uses `to_a2a(root_agent, port=...)` to wrap an ADK
`Agent` into an A2A Starlette app, served by uvicorn.

## Prereqs

- Krypton control plane running (`make dev-up`)
- `GOOGLE_API_KEY` for Gemini

## Build

```bash
docker build -f examples/agent/python/adk/Dockerfile \
  -t krypton/adk-agent:dev examples/agent/python/adk
kind load docker-image --name krypton-dev krypton/adk-agent:dev
```

## Deploy

```bash
kubectl create secret generic adk-secrets \
  -n agents --from-literal=GOOGLE_API_KEY=$GOOGLE_API_KEY
kubectl apply -f examples/agent/python/adk/agent.yaml
```

The Dockerfile honours `PORT` (default `8080`), so [agent.yaml](./agent.yaml)
pins `spec.port: 8080`.

## Invoke

```bash
curl http://localhost:8080/v1/agents/agents/adk/.well-known/agent-card.json

curl -X POST http://localhost:8080/v1/agents/agents/adk/ \
     -H 'Content-Type: application/json' \
     -d '{
       "jsonrpc":"2.0","id":"1","method":"message/send",
       "params":{"message":{"messageId":"1","role":"user",
         "parts":[{"kind":"text","text":"Tell me a fun fact about octopuses."}]}}
     }'
```

## Attribution

Source: <https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/adk_facts>
(Apache 2.0). [`agent.py`](./agent.py), [`__init__.py`](./__init__.py),
[`requirements.txt`](./requirements.txt), [`Dockerfile`](./Dockerfile),
[`.dockerignore`](./.dockerignore), and [`UPSTREAM_README.md`](./UPSTREAM_README.md)
are copied unmodified from upstream.

[adk]: https://google.github.io/adk-docs/
[a2a]: https://a2a-protocol.org/
[upstream]: https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/adk_facts
