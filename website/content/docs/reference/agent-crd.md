---
title: Agent CRD
weight: 1
description: Every field on the Agent spec.
---

`apiVersion: krypton.ai/v1alpha1`, `kind: Agent`, namespaced
(short name `ag`).

## Minimal example

```yaml
apiVersion: krypton.ai/v1alpha1
kind: Agent
metadata:
  name: travel
  namespace: agents
spec:
  image: ghcr.io/org/travel-agent:latest
```

That's the smallest valid Agent. Everything else has a default.

## Full example

```yaml
apiVersion: krypton.ai/v1alpha1
kind: Agent
metadata:
  name: travel
  namespace: agents
spec:
  # Container
  image: ghcr.io/org/travel-agent:latest
  imagePullPolicy: IfNotPresent
  imagePullSecrets:
    - name: ghcr-secret

  # Metadata (informational — surfaced in UI + metrics)
  runtime: python
  framework: langgraph

  # Wire protocol the agent speaks
  protocol: a2a               # a2a | mcp | http

  # Mode (always-on is the supported MVP default)
  mode: always-on             # always-on | serverless (paused)

  # Scaling
  minReplicas: 1
  maxReplicas: 10
  concurrency: 8              # in-flight requests per pod

  # Networking
  port: 8080
  invocationPath: /a2a

  # Pod
  resources:
    requests: { cpu: 100m, memory: 256Mi }
    limits:   { cpu: 1000m, memory: 1Gi }
  env:
    - { name: LOG_LEVEL, value: info }
  envFrom:
    - secretRef: { name: travel-secrets }
  serviceAccountName: ""      # blank = auto-create

  # Lifecycle
  timeout: 60s                # per-invocation
  startupTimeout: 30s         # cold-start grace
```

## Spec reference

### Container

| Field             | Type                            | Default          | Notes                                                                  |
| ----------------- | ------------------------------- | ---------------- | ---------------------------------------------------------------------- |
| `image`           | string (required)               | —                | Container image, including registry + tag                              |
| `imagePullPolicy` | string                          | `IfNotPresent`   | Standard K8s pull policy                                               |
| `imagePullSecrets`| `[]corev1.LocalObjectReference` | —                | Same shape as a pod's `imagePullSecrets`                               |

### Metadata

| Field        | Type   | Default | Notes                                                  |
| ------------ | ------ | ------- | ------------------------------------------------------ |
| `runtime`    | string | —       | Informational (`python`, `node`, `go`, ...)            |
| `framework`  | string | —       | Informational (`langgraph`, `crewai`, ...)             |
| `protocol`   | string | `a2a`   | One of `a2a`, `mcp`, `http`                            |

### Mode + scaling

| Field              | Type            | Default       | Notes                                                                                       |
| ------------------ | --------------- | ------------- | ------------------------------------------------------------------------------------------- |
| `mode`             | string          | `always-on`   | `always-on` (MVP default) or `serverless` (paused — see below)                              |
| `minReplicas`      | int32 (≥ 0)     | `1`           | Always-on floor — pin to 0 only for serverless                                              |
| `maxReplicas`      | int32 (≥ 1)     | `10`          | Must be ≥ `minReplicas`                                                                     |
| `concurrency`      | int32 (≥ 1)     | `8`           | In-flight requests per pod cap (enforced by sidecar)                                        |
| `scaleToZeroAfter` | duration        | `300s`        | Idle window before scale-to-zero — only consulted in `mode: serverless` (paused)            |

### Networking

| Field            | Type            | Default | Notes                                                              |
| ---------------- | --------------- | ------- | ------------------------------------------------------------------ |
| `port`           | int32 (1–65535) | `8080`  | User container's listen port                                       |
| `invocationPath` | string          | `/`     | Path the gateway forwards invocations to (prefix-stripped)         |

### Pod

| Field                | Type                          | Default | Notes                                                |
| -------------------- | ----------------------------- | ------- | ---------------------------------------------------- |
| `resources`          | `corev1.ResourceRequirements` | —       | Same as a pod's `resources`                          |
| `env`                | `[]corev1.EnvVar`             | —       | Passed to user container                             |
| `envFrom`            | `[]corev1.EnvFromSource`      | —       | Same                                                 |
| `serviceAccountName` | string                        | `""`    | Empty = auto-created SA with minimal permissions     |

### Lifecycle

| Field            | Type     | Default | Notes                                                           |
| ---------------- | -------- | ------- | --------------------------------------------------------------- |
| `timeout`        | duration | `60s`   | Bounds a single invocation                                      |
| `startupTimeout` | duration | `30s`   | Activator's cold-start grace; gateway returns 504 if exceeded   |

## Status (read-only)

| Field                | Type      | Written by                  |
| -------------------- | --------- | --------------------------- |
| `phase`              | enum      | Manager                     |
| `replicas`           | int32     | Manager                     |
| `readyReplicas`      | int32     | Manager                     |
| `desiredReplicas`    | int32     | Scaler + activator          |
| `url`                | string    | Manager                     |
| `lastInvocationAt`   | time      | Gateway                     |
| `observedGeneration` | int64     | Manager                     |
| `conditions`         | `[]Cond.` | Manager                     |

### Phases

| Phase     | Meaning                                       |
| --------- | --------------------------------------------- |
| `Pending` | At least one desired replica, none ready yet  |
| `Ready`   | At least one replica ready, OR scaled to zero |
| `Scaling` | (reserved; not currently emitted)             |
| `Failed`  | Persistent reconcile errors (e.g. crashloop)  |

## Validation

Beyond OpenAPI defaults, the (optional) validating webhook enforces:

- `image` non-empty
- `mode: always-on` ⇒ `minReplicas >= 1`
- `concurrency >= 1`
- `maxReplicas >= minReplicas`
- `port` in `[1, 65535]`

Webhooks are off by default (require cert plumbing). The OpenAPI
validation catches everything except the cross-field rules.

## Serverless mode (paused)

`mode: serverless` is implemented but paused in the MVP. To opt in for
an individual agent:

```yaml
spec:
  mode: serverless
  minReplicas: 0
  scaleToZeroAfter: 60s
```

What happens then: the activator catches requests when no pods are
ready, patches `desiredReplicas = 1`, polls `Endpoints` for readiness,
forwards once a pod is up. The scaler drops the agent back to zero
after `scaleToZeroAfter` of idle time. See
[Architecture → Components → Serverless mode (paused)](../../architecture/components/#serverless-mode-paused)
for the current status.
