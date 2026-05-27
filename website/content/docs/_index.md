---
title: Documentation
linkTitle: Documentation
weight: 20
---

Krypton is a Kubernetes-native runtime for AI agents, self-hosted LLMs,
and MCP servers.

For agents, Krypton turns A2A, plain HTTP, and framework-backed
containers into cluster resources with stable gateway routing, lifecycle
management, scaling signals, and observability.

For model serving, Krypton turns a `Model` custom resource into a
llama.cpp deployment, pulls GGUF weights from Hugging Face, and exposes
the result through OpenAI-compatible API paths (`/v1/models`,
`/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`). Your
applications can use familiar SDKs while operators manage models as
ordinary Kubernetes resources.

For MCP, Krypton runs HTTP-transport servers directly or bridges stdio
servers into the same agent gateway and UI introspection path.

## Where to next

- **[Installation](/docs/getting-started/installation/)** — Helm
  install, UI health check, and local access to the gateway
- **[Your first agent](/docs/getting-started/first-agent/)** — bring
  your own LangGraph / ADK / plain HTTP container
- **[Your first LLM](/docs/getting-started/first-llm/)** — deploy a
  Hugging Face GGUF model with llama.cpp and call it with OpenAI SDKs
- **[Your first MCP server](/docs/getting-started/first-mcp/)** — run
  HTTP MCP servers or bridge stdio servers into the cluster
- **[Architecture overview](/docs/architecture/overview/)** — what's
  running and why
- **[Model CRD reference](/docs/reference/model-crd/)** — every field
  on the model-serving API
- **[Reference](/docs/reference/)** — Agent CRD, Helm values, CLI flags
