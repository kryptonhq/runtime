---
title: Tutorials
weight: 2
description: Guided paths for deploying agents, MCP servers, and self-hosted LLMs.
---

These guides are task-oriented. Each one starts from a working Krypton
install, applies a concrete resource, verifies the endpoint, and points
to the reference page for deeper configuration.

## Choose a path

| Goal | Start here |
| ---- | ---------- |
| Deploy an A2A, plain HTTP, ADK, or LangGraph container | [Deploy your first Agent](first-agent/) |
| Host a GGUF model from Hugging Face and call it with an OpenAI SDK | [Deploy your first LLM](first-llm/) |
| Run an MCP server over HTTP or wrap a stdio MCP binary | [Deploy your first MCP server](first-mcp/) |

## Before you begin

Install Krypton first:

```bash
helm install krypton oci://ghcr.io/kryptonhq/charts/krypton \
  --namespace krypton-system --create-namespace
```

For a laptop loop with kind, use [Local testing](../getting-started/local-testing/).
