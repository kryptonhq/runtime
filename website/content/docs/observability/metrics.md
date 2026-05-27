---
title: Metrics
weight: 1
description: Prometheus metrics exposed by every component, plus Prometheus Operator setup.
---

Every Krypton component emits Prometheus metrics. The chart ships
`ServiceMonitor` + `PodMonitor` resources gated behind opt-in flags so
the integration is one `--set` away.

## Quick setup with kube-prometheus-stack

If you don't already have Prometheus Operator on the cluster:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install prom prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace
```

Then enable Krypton's monitors:

```bash
helm upgrade --install krypton oci://ghcr.io/kryptonhq/charts/krypton \
  --namespace krypton-system --create-namespace \
  --set serviceMonitor.enabled=true \
  --set podMonitor.enabled=true \
  --set serviceMonitor.labels.release=prom \
  --set podMonitor.labels.release=prom
```

The `labels.release=prom` matches `kube-prometheus-stack`'s default
`serviceMonitorSelector` (replace `prom` with your stack's release
name). Without it, the operator's Prometheus instance ignores the new
monitors.

Verify scrape targets are up:

```bash
kubectl -n monitoring port-forward svc/prom-kube-prometheus-stack-prometheus 9090:9090
open http://localhost:9090/targets
```

You should see jobs for each Krypton component plus one for the
sidecar across every agent pod.

## Series

### Gateway

| Metric                                  | Type      | Labels                          |
| --------------------------------------- | --------- | ------------------------------- |
| `krypton_invocations_total`             | counter   | `agent`, `namespace`, `status`  |
| `krypton_invocation_duration_seconds`   | histogram | `agent`, `namespace`            |

Histogram buckets: 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, 30s.

### Model serving (gateway)

| Metric                                          | Type      | Labels             |
| ----------------------------------------------- | --------- | ------------------ |
| `krypton_model_invocations_total`               | counter   | `model`, `status`  |
| `krypton_model_invocation_duration_seconds`     | histogram | `model`            |

Emitted for `/v1/chat/completions`, `/v1/completions`, and
`/v1/embeddings` invocations routed via the `Model` CRD. Buckets stretch
out to 120s for long generations: 50ms, 100ms, 250ms, 500ms, 1s, 2.5s,
5s, 10s, 30s, 60s, 120s.

### Scaler (hosted in manager)

| Metric                              | Type    | Labels                                                  |
| ----------------------------------- | ------- | ------------------------------------------------------- |
| `krypton_scaler_decisions_total`    | counter | `agent`, `namespace`, `direction` (`up`/`down`/`noop`)  |
| `krypton_agent_replicas_desired`    | gauge   | `agent`, `namespace`                                    |

### Control plane

| Metric                                  | Type      | Labels                    |
| --------------------------------------- | --------- | ------------------------- |
| `krypton_api_requests_total`            | counter   | `route`, `method`, `code` |
| `krypton_api_request_duration_seconds`  | histogram | `route`                   |

`route` is a route template (`list_agents`, `get_agent`, …), not the
raw URL, so cardinality stays bounded.

### Sidecar (`krypton-proxy`)

| Metric                              | Type    | Labels                              |
| ----------------------------------- | ------- | ----------------------------------- |
| `krypton_proxy_requests_total`      | counter | `agent`, `namespace`, `code`        |
| `krypton_proxy_rejected_total`      | counter | `agent`, `namespace`, `reason`      |
| `krypton_proxy_inflight`            | gauge   | `agent`, `namespace`                |

`reason` is one of `over_capacity` (concurrency cap) or `shutting_down`.

## Grafana dashboard

A starter dashboard lives at
[`deploy/grafana/krypton-overview.json`](https://github.com/kryptonhq/runtime/blob/main/deploy/grafana/krypton-overview.json).

### Import it

1. Grafana → **Dashboards → New → Import**
2. Upload the JSON file (or paste it).
3. Grafana prompts for a value for the `DS_PROM` template input —
   that's where you pick your Prometheus datasource. Select the
   Prometheus that the operator created (default name when installed
   via `kube-prometheus-stack`: **Prometheus**).
4. Click **Import**.

### If the dashboard renders empty / "Datasource DS_PROM was not found"

Means the dashboard JSON references a datasource by its variable name
but Grafana didn't resolve it at import time. Two fixes:

- **Re-import**: delete the dashboard, **New → Import**, pay attention
  to the `DS_PROM` field at the bottom of the dialog and explicitly
  pick your Prometheus datasource before clicking Import.
- **Or edit the panel**: on a broken panel, click ⋯ → **Edit** →
  bottom of the page switch the **Data source** dropdown to your
  Prometheus datasource → **Save**. Repeat per panel, or use the
  dashboard JSON model editor to swap all panels at once.

If you imported the file via a JSON `kind: ConfigMap` (Grafana
sidecar), set the datasource UID explicitly in the JSON before
applying:

```bash
PROM_UID=$(kubectl -n monitoring get datasource prometheus -o jsonpath='{.spec.uid}' 2>/dev/null \
            || echo "prometheus")
sed -i "s/\${DS_PROM}/${PROM_UID}/g" deploy/grafana/krypton-overview.json
kubectl create configmap krypton-overview \
  --from-file=deploy/grafana/krypton-overview.json \
  --namespace monitoring \
  --dry-run=client -o yaml | \
  kubectl label --local -f - grafana_dashboard=1 --dry-run=client -o yaml | \
  kubectl apply -f -
```

### Panels included

- Invocations per second (per agent)
- P95 invocation latency
- Desired replicas
- Scaling decisions per minute (by direction)
- Sidecar in-flight (per pod)
- Sidecar rejected (`over_capacity` vs `shutting_down`)

## Direct port-forward (no Prometheus Operator)

If you're not running the operator but want to eyeball metrics during
local testing:

```bash
# Gateway
kubectl -n krypton-system port-forward deploy/krypton-gateway 8081:8081
curl http://localhost:8081/metrics

# Control plane
kubectl -n krypton-system port-forward deploy/krypton-control-plane 8091:8091
curl http://localhost:8091/metrics

# Manager (controller-runtime + scaler)
kubectl -n krypton-system port-forward deploy/krypton-manager 8080:8080
curl http://localhost:8080/metrics

# Sidecar in an agent pod
kubectl -n agents port-forward deploy/<agent> 8888:8888 -c krypton-proxy
curl http://localhost:8888/metrics
```
