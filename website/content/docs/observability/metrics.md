---
title: Metrics
weight: 1
description: Prometheus series exposed by every component.
---

Every Krypton component emits Prometheus metrics. Three scrape targets
per component (manager, control plane, gateway) on the manager's metrics
port; the sidecar exposes its own `/metrics` on the sidecar port (8888)
per pod.

## Series

### Gateway

| Metric                                  | Type      | Labels                          |
| --------------------------------------- | --------- | ------------------------------- |
| `krypton_invocations_total`             | counter   | `agent`, `namespace`, `status`  |
| `krypton_invocation_duration_seconds`   | histogram | `agent`, `namespace`            |
| `krypton_cold_starts_total`             | counter   | `agent`, `namespace`            |
| `krypton_buffer_depth`                  | gauge     | `agent`, `namespace`            |

Histogram buckets: 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, 30s.

### Scaler (hosted in manager)

| Metric                              | Type    | Labels                                              |
| ----------------------------------- | ------- | --------------------------------------------------- |
| `krypton_scaler_decisions_total`    | counter | `agent`, `namespace`, `direction` (`up`/`down`/`noop`) |
| `krypton_agent_replicas_desired`    | gauge   | `agent`, `namespace`                                |

### Control plane

| Metric                                  | Type      | Labels                          |
| --------------------------------------- | --------- | ------------------------------- |
| `krypton_api_requests_total`            | counter   | `route`, `method`, `code`       |
| `krypton_api_request_duration_seconds`  | histogram | `route`                         |

`route` is a route template (`list_agents`, `get_agent`, ...), not the
raw URL, so cardinality stays bounded.

### Sidecar (`krypton-proxy`)

| Metric                              | Type    | Labels                              |
| ----------------------------------- | ------- | ----------------------------------- |
| `krypton_proxy_requests_total`      | counter | `agent`, `namespace`, `code`        |
| `krypton_proxy_rejected_total`      | counter | `agent`, `namespace`, `reason`      |
| `krypton_proxy_inflight`            | gauge   | `agent`, `namespace`                |

`reason` is one of `over_capacity` (concurrency cap) or `shutting_down`.

## Grafana

A starter dashboard is available at
[`deploy/grafana/krypton-overview.json`](https://github.com/kryptonhq/runtime/blob/main/deploy/grafana/krypton-overview.json).
Import it in **Dashboards → New → Import**, pick your Prometheus
datasource for the `DS_PROM` variable.

Panels:

- Invocations per second (per agent)
- P95 invocation latency
- Cold starts per minute
- Desired replicas
- Buffer depth
- Scaling decisions per minute (by direction)
- Sidecar in-flight (per pod)

## OpenTelemetry / tracing

Not in MVP. Will be added as a follow-up — the metric series above plus
structured logs (`agent_name`, `invocation_id`, `trace_id`) cover the
80% case for now.
