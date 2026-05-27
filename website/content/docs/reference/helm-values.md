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
  tag: {{< version >}}

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
  registry: ghcr.io/kryptonhq
  tag: ""              # empty → falls back to .Chart.AppVersion
  pullPolicy: IfNotPresent
```

Leaving `tag` empty makes the chart pin images at the same version as
the chart release (the helper falls back to `.Chart.AppVersion`), so
`helm install` Just Works without explicit `--set image.tag=...`.

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

### `serviceMonitor`

Prometheus Operator integration for the three runtime components
(manager, control-plane, gateway). Off by default so the chart
installs cleanly on clusters without the prometheus-operator CRDs.

```yaml
serviceMonitor:
  enabled: false
  namespace: ""        # default: release namespace
  labels: {}           # match these against Prometheus's serviceMonitorSelector
  interval: 30s
  scrapeTimeout: 10s
  relabelings: []
  metricRelabelings: []
```

When the cluster runs `kube-prometheus-stack`, the typical install is:

```bash
helm install krypton oci://ghcr.io/kryptonhq/charts/krypton \
  --namespace krypton-system --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.labels.release=prom
```

The `labels.release=prom` matches the default `serviceMonitorSelector`
applied by `kube-prometheus-stack` (replace `prom` with your stack's
release name).

### `podMonitor`

Same idea, for the `krypton-proxy` sidecar that gets injected into
every Agent pod. Sidecars live in dynamic agent namespaces and have
no Services of their own, so a `PodMonitor` (matching on the
`app.kubernetes.io/managed-by: krypton-runtime` label) is the right
shape.

```yaml
podMonitor:
  enabled: false
  namespace: ""
  labels: {}
  namespaceSelector: {} # default: any namespace
                        # restrict via: { matchNames: [agents, prod-agents] }
  interval: 30s
  scrapeTimeout: 10s
```
