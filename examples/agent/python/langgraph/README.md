# LangGraph + A2A on Krypton

A currency-conversion agent built with [LangGraph][langgraph] and exposed
via the [A2A protocol][a2a]. Copied verbatim from
[a2aproject/a2a-samples][upstream] — see [UPSTREAM_README.md](./UPSTREAM_README.md)
for the original walkthrough.

This wrapper adds a Krypton `Agent` CR and a build/deploy recipe; the
agent code itself is unchanged.

## Prereqs

- Krypton control plane running (`make dev-up`)
- A Gemini API key (`GOOGLE_API_KEY`) **or** an OpenAI-compatible endpoint
  (see upstream README)

## Build

The sample ships a [Containerfile](./Containerfile). The upstream entrypoint
binds to port `10000` — the [agent.yaml](./agent.yaml) below points
Krypton at the same port.

```bash
docker build -f examples/agent/python/langgraph/Containerfile \
  -t krypton/langgraph-agent:dev examples/agent/python/langgraph
kind load docker-image --name krypton-dev krypton/langgraph-agent:dev
```

## Deploy

```bash
kubectl create secret generic langgraph-secrets \
  -n agents --from-literal=GOOGLE_API_KEY=$GOOGLE_API_KEY
kubectl apply -f examples/agent/python/langgraph/agent.yaml
```

## Invoke

```bash
# Discover the agent card
curl http://localhost:8080/v1/agents/agents/langgraph/.well-known/agent-card.json

# Send a message (A2A JSON-RPC)
curl -X POST http://localhost:8080/v1/agents/agents/langgraph/ \
     -H 'Content-Type: application/json' \
     -d '{
       "jsonrpc":"2.0","id":"1","method":"message/send",
       "params":{"message":{"messageId":"1","role":"user",
         "parts":[{"kind":"text","text":"What is the exchange rate between USD and GBP?"}]}}
     }'
```

## Attribution

Source: <https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/langgraph>
(Apache 2.0). All files under [`app/`](./app), [`pyproject.toml`](./pyproject.toml),
[`Containerfile`](./Containerfile), and [`UPSTREAM_README.md`](./UPSTREAM_README.md)
are copied unmodified from upstream.

[langgraph]: https://langchain-ai.github.io/langgraph/
[a2a]: https://a2a-protocol.org/
[upstream]: https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/langgraph
