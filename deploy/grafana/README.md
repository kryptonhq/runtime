# Grafana dashboards

`krypton-overview.json` — runtime overview: invocation rate, P95 latency,
cold starts, desired replicas, buffer depth, scaling decisions, and
sidecar in-flight gauges.

## Importing

1. In Grafana, **Dashboards → New → Import**, paste the JSON.
2. Pick your Prometheus datasource for the `DS_PROM` variable.

## Where the metrics come from

| Metric prefix | Scraped from |
| ------------- | ------------ |
| `krypton_invocations_*`, `krypton_cold_starts_*`, `krypton_buffer_depth` | gateway (`cmd/gateway`) |
| `krypton_scaler_*`, `krypton_agent_replicas_desired` | manager (`cmd/manager`) |
| `krypton_api_*` | control plane (`cmd/control-plane`) |
| `krypton_proxy_*` | sidecar (`cmd/krypton-proxy`) — per-pod scrape |

All except the sidecar register with controller-runtime's metrics
registry and are exposed on each component's `--metrics-bind-address`.
The sidecar exposes its own `/metrics` on the sidecar port (8888 by
default).
