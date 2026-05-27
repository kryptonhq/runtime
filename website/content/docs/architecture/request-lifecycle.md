---
title: Request lifecycle
weight: 3
description: What happens between curl and JSON.
---

This page walks through what happens between `curl POST .../invocations`
and the JSON coming back.

## Hot path (pod already running)

```
1. client → gateway:8080
       POST /v1/agents/agents/echo/foo
                                      ──────────
                                      parsed as agent=echo,
                                      namespace=agents,
                                      rest=/foo

2. gateway.Activator.Resolve(echo)
       ├── fetch Agent CR from informer cache
       ├── Check Endpoints — has ready addresses ✓
       └── return http://echo.agents.svc:8080/  (hot path, no cold start)

3. gateway.handleInvocation
       └── httputil.ReverseProxy forwards
             ├── path rewritten to /foo
             ├── traceparent header preserved
             └── FlushInterval = -1 (immediate flush, streaming-safe)

4. kube-proxy → Endpoint IP → krypton-proxy:8888

5. krypton-proxy
       ├── shuttingDown? no
       ├── acquire concurrency slot (non-blocking)
       │     ├── if cap reached → 503 + Retry-After (back to client)
       │     └── otherwise → in-flight++ , inflight gauge++
       └── reverse-proxy to 127.0.0.1:<spec.port>

6. user container handles the request
       └── response streams back through:
             user → proxy → kube-proxy → gateway → client

7. gateway.handleInvocation completes
       └── go RecordInvocation(...)
             └── patches Agent.Status.LastInvocationAt = now
                  (decoupled from request ctx so it survives disconnect)
```

Typical latency: P50 ~50ms, P95 ~200ms for a 100ms user-handler.

## Scale-up under load

```
T+0:     burst arrives. Sidecars start refusing past spec.concurrency
         with 503 + Retry-After.
T+1s:    scaler tick. inflight = N (sum of sidecar reports).
         desired = ceil(N / concurrency), clamped to [minReplicas, maxReplicas].
         If higher than current, scaler patches Status.DesiredReplicas.

T+1s+ε:  Manager reconciler observes change
         → patches Deployment.Spec.Replicas to the new desired
         → K8s creates additional pods
         → new sidecars take traffic
```

## Scale-down

```
T+0:    load drops. inflight returns to baseline.
T+1s+:  scaler tick — desired computed below current.
        If within scaler-stable-window-ms (default 60s) of the last
        scale-up, hold. Otherwise patch Status.DesiredReplicas down.
        Always-on never goes below max(minReplicas, 1).
```

The sample MCP server in [`examples/mcp/go`](https://github.com/kryptonhq/runtime/tree/main/examples/mcp/go)
demonstrates clean SIGTERM handling for when pods do get terminated —
without it, the pod would exit with status 143 (`Error`) instead of
`Completed`.

## Hysteresis on bursty load

The scaler refuses to scale down within `--scaler-stable-window-ms`
(60s default) of the most recent scale-up. Burst arrives, scales up;
load drops momentarily; scaler holds replica count; load returns —
the runtime didn't churn through pod terminations.

## Cold path (serverless — paused)

The activator implements scale-from-zero for `mode: serverless` agents:
buffered request → patch `desiredReplicas = 1` + `lastInvocationAt` →
poll `Endpoints` until ready → forward. This code path stays in the
binary (it has unit + integration coverage) but isn't the recommended
deployment model for MVP. See
[Components → Serverless mode (paused)](../components/#serverless-mode-paused).
