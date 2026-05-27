---
title: Helm values
weight: 3
description: Chart values reference.
---

Chart source: [`deploy/helm/krypton`](https://github.com/kryptonhq/runtime/tree/main/deploy/helm/krypton).

## Common overrides

```yaml
# values.yaml
image:
  registry: ghcr.io/kryptonhq
  tag: v0.1.0

controlPlane:
  databaseUrl: "postgres://user:pass@host:5432/db"

manager:
  enableWebhooks: false
  enableScaler: true

gateway:
  service:
    type: ClusterIP   # operators add their own ingress in front
```

## Full reference

### `image`

Default image base for all components. Per-component overrides under
`images.*` take precedence.

```yaml
image:
  registry: krypton
  tag: dev
  pullPolicy: IfNotPresent
```

### `images.*`

Per-component image overrides — each accepts `repository`, `tag`,
`pullPolicy`.

```yaml
images:
  manager:      { repository: my-registry/manager, tag: custom }
  controlPlane: {}
  gateway:      {}
  proxy:        {}   # used by manager via --proxy-image
```

### `manager`

```yaml
manager:
  replicas: 1
  enableWebhooks: false
  enableScaler: true
  scalerIntervalMs: 1000
  scalerStableWindowMs: 60000
  resources:
    requests: { cpu: 100m, memory: 128Mi }
    limits:   { cpu: 500m, memory: 256Mi }
```

### `controlPlane`

```yaml
controlPlane:
  replicas: 1
  databaseUrl: ""           # empty = in-memory store
  service:
    type: ClusterIP
    port: 8090
  resources:
    requests: { cpu: 50m, memory: 64Mi }
    limits:   { cpu: 500m, memory: 256Mi }
```

### `gateway`

```yaml
gateway:
  replicas: 1
  maxBufferPerAgent: 100
  pollIntervalMs: 50
  defaultStartupTimeoutMs: 30000
  service:
    type: ClusterIP
    port: 8080
  resources:
    requests: { cpu: 50m, memory: 64Mi }
    limits:   { cpu: 500m, memory: 256Mi }
```

### `postgres` (optional bundled instance)

For dev installs. Production should use a managed instance and pass
`controlPlane.databaseUrl`.

```yaml
postgres:
  enabled: false
  image:
    repository: postgres
    tag: "16-alpine"
  auth:
    user: krypton
    password: krypton
    database: krypton
  persistence:
    enabled: false
    size: 1Gi
```

When `postgres.enabled: true` and `controlPlane.databaseUrl` is empty,
the chart auto-wires the control plane to the bundled instance via the
generated service DNS.

### `rbac`

```yaml
rbac:
  create: true   # set false if you bring your own
```
