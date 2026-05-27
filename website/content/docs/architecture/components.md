---
title: Components
weight: 2
description: Per-component deep dive.
---

## Manager

Source: [`cmd/manager`](https://github.com/kryptonhq/runtime/tree/main/cmd/manager) +
[`internal/controller`](https://github.com/kryptonhq/runtime/tree/main/internal/controller).

Standard controller-runtime operator. Reconciles `Agent` CRs by owning
three child resources per agent: a `Deployment`, a `Service`, and a
`ServiceAccount`. The Deployment is injected with the `krypton-proxy`
sidecar at template-render time.

Also runs the [scaling decider](#scaler) as a `manager.Runnable`.

Key behavior:

- **`CreateOrUpdate` semantics**: each child resource uses
  `controllerutil.CreateOrUpdate` wrapped in `retry.RetryOnConflict` so
  spec drift converges without hot-looping when the apps controller
  concurrently updates Deployment status.
- **Status writes use `Patch` with `MergeFrom`**, not `Update`, so they
  don't conflict with the scaler/gateway's concurrent writes to other
  status fields.
- **Finalizer** `krypton.ai/cleanup` blocks deletion until child
  resources have drained.

## Control plane

Source: [`cmd/control-plane`](https://github.com/kryptonhq/runtime/tree/main/cmd/control-plane) +
[`internal/controlplane`](https://github.com/kryptonhq/runtime/tree/main/internal/controlplane).

A controller-runtime manager with no reconcilers — just a cache that
watches `Agent` CRs across the cluster. Serves the public REST API from
that cache (always fresh, no DB hop):

| Path                                          | Returns                            |
| --------------------------------------------- | ---------------------------------- |
| `GET /v1/agents[?namespace=...]`              | List agents                        |
| `GET /v1/agents/{namespace}/{name}`           | Single agent                       |
| `GET /v1/agents/{namespace}/{name}/status`    | Just the status subresource        |
| `GET /healthz`, `/readyz`                     | Probes                             |
| `GET /ui/*`                                   | Embedded React UI                  |

When `--database-url` (or `$DATABASE_URL`) is set, an additional
`Syncer` Reconciler mirrors each Agent CR into Postgres on every event.

## Gateway

Source: [`cmd/gateway`](https://github.com/kryptonhq/runtime/tree/main/cmd/gateway) +
[`internal/gateway`](https://github.com/kryptonhq/runtime/tree/main/internal/gateway).

Public ingress. Any request to `/v1/agents/{namespace}/{name}[/...]`
is reverse-proxied to the agent's in-cluster Service via
`httputil.ReverseProxy` with `FlushInterval = -1` — SSE / chunked HTTP
arrive at the client as the upstream produces them, not at EOF.

The gateway strips exactly `/v1/agents/{namespace}/{name}` and
forwards the rest of the path verbatim. Agents see `/`,
`/.well-known/agent-card.json`, `/oauth/callback`, or whatever else
they implement — no knowledge of the gateway prefix required.

After each successful invocation, the gateway asynchronously patches
`status.lastInvocationAt` (decoupled from the request context via
`context.WithoutCancel` so the patch survives client disconnect).

### Serverless mode (paused)

The gateway also contains an **activator**: a per-agent bounded buffer
that catches requests when no pods are ready, patches
`status.desiredReplicas = 1`, and polls `Endpoints` until the
cold-started pod becomes ready. The code path is tested and functional;
it's hidden from the MVP because cold-start + scale-to-zero needs more
end-to-end tuning before we recommend it.

To opt in for an individual agent, set `mode: serverless` and
`minReplicas: 0` explicitly on the `Agent` CR. The activator's bounded
buffer (default 100 waiters per agent) and 30s cold-start timeout
behave as documented in [`internal/gateway/activator.go`](https://github.com/kryptonhq/runtime/blob/main/internal/gateway/activator.go).

## Scaler

Source: [`internal/scaler`](https://github.com/kryptonhq/runtime/tree/main/internal/scaler).

Hosted by the manager process. Ticks every `--scaler-interval-ms`
(default 1s) and for each `Agent`:

1. Queries each ready pod IP from `Endpoints` for its sidecar's
   `/_krypton/inflight` count, sums them
2. Computes `desired = clamp(ceil(inflight / concurrency), min, max)`
3. Always-on floor: `max(minReplicas, 1)` — never scales below this
4. **Hysteresis**: refuses to scale down within `--scaler-stable-window-ms`
   (default 60s) of the most recent scale-up. Prevents flapping under
   bursty load.

(Serverless-mode scale-to-zero is implemented in the same decider but
not enabled by the default `mode: always-on`. See [Serverless mode (paused)](#serverless-mode-paused).)

## Sidecar (`krypton-proxy`)

Source: [`cmd/krypton-proxy`](https://github.com/kryptonhq/runtime/tree/main/cmd/krypton-proxy) +
[`internal/sidecar`](https://github.com/kryptonhq/runtime/tree/main/internal/sidecar).

Injected next to every Agent container. Listens on port `8888`,
forwards to the user container on `spec.port`.

| Endpoint                  | Purpose                                                                                         |
| ------------------------- | ----------------------------------------------------------------------------------------------- |
| `/healthz`                | Always 200 (liveness)                                                                           |
| `/readyz`                 | 200 normally; 503 during graceful shutdown                                                     |
| `/metrics`                | Prometheus — `krypton_proxy_requests_total`, `krypton_proxy_inflight`, `krypton_proxy_rejected_total` |
| `/_krypton/inflight`      | JSON: in-flight count, last-activity ns, concurrency cap                                       |
| anything else             | Reverse-proxied to user container                                                              |

Concurrency is enforced via a non-blocking semaphore — over the cap
returns 503 + `Retry-After` immediately. Graceful shutdown drains
in-flight requests up to `KRYPTON_SHUTDOWN_TIMEOUT` (default 25s).

The Service routes external traffic to the **sidecar port** (`TargetPort
= proxy`), not directly to the user container.
