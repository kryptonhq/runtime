---
title: Architecture
linkTitle: Architecture
weight: 1
description: Components and state model.
aliases:
  - /docs/architecture/
  - /docs/architecture/overview/
---

Krypton is composed of four binaries:

| Component        | Role                                                                                                                  |
| ---------------- | --------------------------------------------------------------------------------------------------------------------- |
| **Manager**      | Kubernetes operator. Reconciles `Agent` CRs → Deployments + Services + ServiceAccounts. Runs the scaling decider.    |
| **Control plane**| Read-only HTTP API + the operator UI. Optionally mirrors agents into Postgres for offline querying.                  |
| **Gateway**      | Public ingress. Reverse-proxies invocations to the agent's in-cluster Service.                                       |
| **Sidecar**      | Per-pod `krypton-proxy`. Enforces concurrency, surfaces in-flight count, exposes Prometheus metrics.                 |

## High-level diagram

```mermaid
%%{init: {"theme": "base", "flowchart": {"nodeSpacing": 70, "rankSpacing": 80, "diagramPadding": 24}, "themeVariables": {"fontFamily": "Inter, ui-sans-serif, system-ui, sans-serif", "primaryColor": "#eef2ff", "primaryTextColor": "#1f2937", "primaryBorderColor": "#6366f1", "lineColor": "#64748b", "secondaryColor": "#ecfeff", "tertiaryColor": "#f8fafc"}}}%%
flowchart TB
    client["Client"]
    ui["Krypton UI<br/>Operator console"]
    cp["Control plane<br/>REST API and cache"]
    gw["Gateway<br/>Ingress and activator"]
    mgr["Manager<br/>Controller"]
    scaler["Scaler<br/>Replica decisions"]

    subgraph pod["Agent pod"]
      proxy["krypton-proxy<br/>Concurrency and metrics"]
      app["User agent container"]
      proxy -->|"proxy"| app
    end

    ui -->|"REST"| cp
    client -->|"invoke"| gw
    gw -->|"proxy request"| proxy
    cp -->|"watch"| mgr
    mgr -->|"owns"| pod
    scaler -->|"scale"| pod
    scaler -. "in-flight" .-> proxy

    classDef external fill:#f8fafc,stroke:#94a3b8,color:#0f172a;
    classDef control fill:#eef2ff,stroke:#6366f1,color:#312e81;
    classDef traffic fill:#ecfeff,stroke:#0891b2,color:#164e63;
    classDef runtime fill:#f0fdf4,stroke:#16a34a,color:#14532d;
    classDef podbox fill:#ffffff,stroke:#cbd5e1,color:#0f172a;
    class client external;
    class ui,cp,mgr,scaler control;
    class gw,proxy traffic;
    class app runtime;
    class pod podbox;
```

## Where state lives

| State                          | Source of truth                              |
| ------------------------------ | -------------------------------------------- |
| Agent desired spec             | The `Agent` CR (Kubernetes etcd)             |
| `status.phase`, `replicas`     | Manager writes; readers consume              |
| `status.desiredReplicas`       | Scaler (in manager)                          |
| `status.lastInvocationAt`      | Gateway writes after each invocation         |
| In-flight count                | Sidecar's `/_krypton/inflight` endpoint      |
| Invocation history (later)     | Postgres                                     |

**CRDs are the source of truth.** Postgres is a write-through mirror —
the API serves directly from the informer cache (fresher, no DB hop).

{{% alert title="Always-on mode" color="info" %}}
Krypton runs every agent in **always-on** mode by default —
`minReplicas: 1` keeps one pod warm per agent. Scale-from-zero
support is on the [roadmap](/docs/roadmap/).
{{% /alert %}}

## Next

For what each component does, how they're wired, and the request
lifecycle through them, see [Components](../components/) and
[Request lifecycle](../request-lifecycle/).
