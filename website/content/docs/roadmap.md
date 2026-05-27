---
title: Roadmap
weight: 6
description: What's planned next.
---

A forward-looking view of where Krypton is headed. Order suggests rough
priority; nothing is committed and dates aren't promises.

## Near term

| Item                          | Status      | Notes                                                                 |
| ----------------------------- | ----------- | --------------------------------------------------------------------- |
| OpenTelemetry tracing         | Planned     | OTLP exporter across gateway → activator → pod; W3C traceparent propagation |
| MCP resources + prompts       | Planned     | `resources/list`, `resources/read`, `prompts/list`, `prompts/get`     |
| Schema-driven MCP tool forms  | Planned     | Replace JSON textarea with auto-generated inputs per tool             |
| Invocation history (Postgres) | Planned     | Surfaced in UI; queryable via control plane                           |
| TCP-dial readiness check      | Planned     | Closes the kube-proxy programming race on first cold-start            |
| Cert-manager integration      | Planned     | One-shot webhook enablement; today requires manual cert plumbing      |

## Medium term

| Item                          | Status      | Notes                                                                 |
| ----------------------------- | ----------- | --------------------------------------------------------------------- |
| Serverless mode (GA)          | Paused      | Code is functional; needs more end-to-end tuning before recommending  |
| GPU-aware scheduling          | Planned     | `spec.gpu: { count, type }` → nodeSelector / tolerations; MIG slicing |
| AI-native observability       | Planned     | OTEL GenAI semantic conventions, token usage roll-ups                 |
| Per-agent network policies    | Planned     | Generated egress / ingress rules from spec                            |
| Secure sandbox runtime        | Planned     | `Sandbox` CRD with gVisor / Kata runtime classes for AI coding agents |

## Longer term

| Item                          | Status      | Notes                                                                 |
| ----------------------------- | ----------- | --------------------------------------------------------------------- |
| Multi-cluster federation      | Exploring   | Federated control plane; cross-cluster routing with failover          |
| Authentication & multi-tenancy| Exploring   | OIDC-backed UI/API, per-tenant namespaces with quota                  |
| Image provenance              | Exploring   | Sigstore / cosign signature verification at admission                 |
| `Sign in with Vercel` SSO     | Exploring   | OAuth into the operator UI                                            |

## Out of scope

Some things people ask about that we deliberately won't build:

| Item                          | Why not                                                               |
| ----------------------------- | --------------------------------------------------------------------- |
| Built-in model hosting        | Krypton runs **agents**, not models. Use any provider.                |
| A vector DB                   | Agents bring their own (managed Pinecone, pgvector, Weaviate, …).     |
| Prompt management             | Belongs in agent code or a sibling product.                           |
| Non-Kubernetes deployment     | The design hard-relies on the Kubernetes API for desired-state, scheduling, and lifecycle. |

Have a feature in mind that's not listed? Open an
[issue](https://github.com/kryptonhq/runtime/issues).
