# Google ADK-Go + A2A on Krypton

A prime-number-checking agent built with the [Go ADK][adk-go] and
exposed via the [A2A protocol][a2a]. The agent code
([`main.go`](./main.go)) is copied verbatim from the official
[ADK-Go A2A quickstart][upstream] — see
[UPSTREAM_README.md](./UPSTREAM_README.md) for the original walkthrough.

The upstream sample uses the ADK web launcher with `a2a.NewLauncher()`,
which dynamically generates the agent card and serves JSON-RPC at
`/`. It hardcodes port `8001`.

## Prereqs

- Krypton control plane running (`make dev-up`)
- A Gemini API key (`GOOGLE_API_KEY`)

## Build

```bash
docker build -f examples/agent/go/Dockerfile \
  -t krypton/adk-go-agent:dev examples/agent/go
kind load docker-image --name krypton-dev krypton/adk-go-agent:dev
```

## Deploy

```bash
kubectl create secret generic adk-go-secrets \
  -n agents --from-literal=GOOGLE_API_KEY=$GOOGLE_API_KEY
kubectl apply -f examples/agent/go/agent.yaml
```

## Invoke

```bash
curl http://localhost:8080/v1/agents/agents/adk-go/.well-known/agent-card.json

curl -X POST http://localhost:8080/v1/agents/agents/adk-go/ \
     -H 'Content-Type: application/json' \
     -d '{
       "jsonrpc":"2.0","id":"1","method":"message/send",
       "params":{"message":{"messageId":"1","role":"user",
         "parts":[{"kind":"text","text":"Are 7, 12, and 29 prime?"}]}}
     }'
```

## Attribution

Source: <https://github.com/google/adk-docs/tree/main/examples/go/a2a_basic/remote_a2a/check_prime_agent>
(Apache 2.0). [`main.go`](./main.go), [`go.mod`](./go.mod),
[`go.sum`](./go.sum), and [`UPSTREAM_README.md`](./UPSTREAM_README.md)
are copied unmodified from upstream (the module name in `go.mod` is
re-pathed for this repo).

[adk-go]: https://github.com/google/adk-go
[a2a]: https://a2a-protocol.org/
[upstream]: https://adk.dev/a2a/quickstart-exposing-go/
