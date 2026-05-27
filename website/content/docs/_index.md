---
title: Documentation
linkTitle: Documentation
weight: 20
description: Start here for Krypton concepts, tutorials, operations, and reference.
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

## Start by intent

| If you want to... | Go here |
| ----------------- | ------- |
| Install Krypton and confirm the gateway and UI are healthy | [Getting started](getting-started/) |
| Deploy a real workload step by step | [Tutorials](tutorials/) |
| Understand the resource model and request path | [Concepts](concepts/) |
| Configure ingress, metrics, and production troubleshooting | [Operations](operations/) |
| Look up exact fields, flags, and chart values | [Reference](reference/) |

## Fast path

1. [Install Krypton](getting-started/installation/).
2. [Deploy your first Agent](tutorials/first-agent/) or
   [deploy your first LLM](tutorials/first-llm/).
3. [Read the architecture](concepts/architecture/) when you are ready
   to tune routing, scaling, or operations.
