---
title: Roadmap
weight: 6
description: What's planned next.
---

A forward-looking view of where Krypton is headed. Order within each
bucket suggests rough priority; nothing is a commitment and dates
aren't promises.

## Recently shipped

| Item                                | Notes                                                                 |
| ----------------------------------- | --------------------------------------------------------------------- |
| Built-in model hosting (`Model` CRD)| Host LLMs by Hugging Face name via [llama.cpp][llamacpp], aggregated behind one OpenAI-compatible endpoint. See [first LLM](/docs/getting-started/first-llm/). |

## High priority

| Item                          | Notes                                                                 |
| ----------------------------- | --------------------------------------------------------------------- |
| GPU-aware scheduling          | `spec.gpu` on `Model` / `Agent`; multi-GPU and MIG slicing            |
| More inference runtimes       | vLLM and TGI behind the same `Model` CRD                              |
| Model autoscaling             | Concurrency-aware scaling for `Model` pods (today they're always-on)  |
| Secure sandbox runtime        | `Sandbox` CRD with gVisor / Kata runtime classes                      |
| AI-native observability       | OTEL GenAI conventions; token-usage roll-ups                          |
| OpenTelemetry tracing         | OTLP exporter; `traceparent` propagation across hops                  |

## Medium priority

| Item                          | Notes                                                                 |
| ----------------------------- | --------------------------------------------------------------------- |
| Serverless mode (GA)          | Cold-start activator path, off by default today                       |
| Invocation history (Postgres) | Queryable history, surfaced in UI                                     |
| Schema-driven MCP tool forms  | Auto-generated inputs per tool from its JSON schema                   |
| MCP resources + prompts       | `resources/*` and `prompts/*` method support                          |
| Per-agent network policies    | Egress / ingress rules generated from spec                            |
| TCP-dial readiness check      | Closes the kube-proxy race on first cold-start                        |

## Exploring

| Item                          | Notes                                                                 |
| ----------------------------- | --------------------------------------------------------------------- |
| Multi-cluster federation      | Federated control plane; cross-cluster routing                        |
| Authentication & multi-tenancy| OIDC-backed UI/API; per-tenant namespaces                             |

## Out of scope

Things people ask about that we deliberately won't build:

| Item                          | Why not                                                               |
| ----------------------------- | --------------------------------------------------------------------- |
| A vector DB                   | Use managed (Pinecone, pgvector, Weaviate, …)                         |
| Prompt management             | Belongs in agent code                                                 |
| Cert-manager integration      | Lives at the ingress layer                                            |
| Non-Kubernetes deployment     | Design hard-relies on the Kubernetes API                              |

Have a feature in mind that's not listed? Open an
[issue](https://github.com/kryptonhq/runtime/issues).

[llamacpp]: https://github.com/ggerganov/llama.cpp
