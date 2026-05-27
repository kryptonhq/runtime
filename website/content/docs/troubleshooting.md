---
title: Troubleshooting & FAQ
linkTitle: Troubleshooting
weight: 5
description: Common failure modes, fixes, and frequently-asked questions.
---

## Troubleshooting

### Pods can't pull images (`ImagePullBackOff`)

In kind, confirm the image was loaded:

```bash
kind load docker-image --name krypton-dev <image>:<tag>
```

Image policy must allow local images:

```yaml
imagePullPolicy: IfNotPresent   # or Never for strict local-only
```

### Cold start times out (504)

The gateway timed out waiting for an Endpoint to become ready. Common
causes:

| Cause                                  | What to check                                      |
| -------------------------------------- | -------------------------------------------------- |
| Image pull is slow (uncached)          | `kubectl describe pod -n <ns> <pod>`               |
| Agent container fails readiness        | `kubectl logs -n <ns> <pod> -c agent`              |
| Sidecar `/readyz` failing              | `kubectl logs -n <ns> <pod> -c krypton-proxy`      |
| Wrong `spec.port` (agent listens elsewhere) | Look for "connect: connection refused" in sidecar logs |

Bump the timeout if your agent is slow to start:

```yaml
spec:
  startupTimeout: 90s
```

or globally via the gateway flag `--default-startup-timeout-ms`.

### UI shows "not built"

The control plane image was compiled without `make ui` running first.
The bundled `hack/local-up.sh` does this for you; if you build images
by hand, run:

```bash
make ui
make docker-build
```

### Agent stays at `Pending` forever

Inspect the underlying Deployment:

```bash
kubectl -n <ns> describe deploy <agent>
kubectl -n <ns> get events --sort-by=.lastTimestamp | tail
```

Common reasons: image pull errors, sidecar probe failing, resource
limits too tight, scheduler can't find a node.

### Pod scales to zero immediately after coming up

This shouldn't happen with current code, but is the symptom of:

- Activator failed to patch `Status.LastInvocationAt` at cold-start
  time — check the gateway image is current
- Scaler is running with a too-short stable window

Verify the agent's recent status:

```bash
kubectl -n <ns> get agent <name> -o jsonpath='{.status.lastInvocationAt}'
```

It should be a fresh timestamp right after invocation.

### Pod terminates with `Error` instead of `Completed`

The user container isn't handling SIGTERM. Add a signal handler that
calls `Server.Shutdown()` and exits 0. The bundled
[`examples/mcp/go`](https://github.com/kryptonhq/runtime/blob/main/examples/mcp/go/main.go)
shows the minimum pattern.

### `connection refused` on the very first cold-start invocation

There's a brief window between "Endpoints object has ready addresses"
and "kube-proxy has programmed the iptables rules". The gateway can hit
that gap on the first request after a fresh pod.

Workaround: retry once. A proper fix (TCP-dial verification before
returning from the activator) is a tracked follow-up.

### Hot loop of reconciler logs (`reconciled phase=Ready` 100×/s)

If you're forking the controller and hit this, you're sending full-object
`Update`s that strip metadata (managedFields, annotations), causing
the apiserver to bump resourceVersion every reconcile and re-trigger
the owner watch. Use `controllerutil.CreateOrUpdate` (which mutates the
fetched object) wrapped in `retry.RetryOnConflict`.

### "Operation cannot be fulfilled on agents.krypton.ai" conflict errors

The reconciler and scaler both want to write to `agent.Status`. Use
`Status().Patch` with `client.MergeFrom`, not `Status().Update`, so the
patch only carries fields you actually modified and other writers don't
conflict.

## FAQ

### Is Krypton production-ready?

Pre-alpha. APIs (CRD, REST) are unstable. We're running the runtime
internally; the public surface will firm up once we have external
adopters.

### Does Krypton need a model gateway / LLM provider?

No. Krypton runs **agents**, which themselves call models. Bring any
LLM provider you want — OpenAI, Anthropic, Vercel AI Gateway, a
self-hosted server — your agent code does the calling.

### What protocols are supported?

`spec.protocol: a2a | mcp | http`. A2A and MCP get first-class treatment
(MCP gets tool introspection in the UI). `http` is the escape hatch for
anything else — the gateway forwards bytes; you decide what they mean.

### Can I run multiple replicas behind a single Agent?

Yes — that's the default. Set `spec.maxReplicas`. The scaler maintains
`ceil(inflight / concurrency)` replicas up to that cap. The sidecar
enforces `spec.concurrency` per pod so no individual replica gets
overloaded.

### How is this different from Knative?

Knative is HTTP-centric and assumes one container per Service. Krypton
models A2A and MCP as first-class — the CRD speaks their language
(tool introspection, session affinity, agent lifecycle), and the
control plane / UI surface them as agents rather than generic
workloads.

We borrow heavily from Knative's design (Activator pattern,
KPA-style scaling) without depending on it.

### Do I need to install a service mesh?

No. Krypton uses plain Kubernetes Services + the
`krypton-proxy` sidecar. If you already run Istio / Linkerd /
Cilium, the sidecar coexists with their data plane.

### Can the gateway handle TLS?

Not directly — Krypton ships as ClusterIP. Operators put their existing
ingress (Envoy, Nginx, ALB, Cloudflare, Gateway API) in front for TLS
termination. See [Installation](/docs/getting-started/installation/#bring-your-own-ingress).

### What happens if the agent's container crashes?

Standard Kubernetes restart semantics. The Deployment recreates the
pod; the sidecar's readiness probe gates traffic until the user
container is healthy.

### Where do invocations get logged?

Today: structured logs on each component's stdout (gateway logs
invocations, sidecar logs concurrency events). Future: Postgres-backed
invocation history once the schema settles.

### Does Krypton support serverless (scale-to-zero)?

The code is there but turned off by default — see
[architecture/components](/docs/architecture/components/#serverless-mode-paused).
Opt in per-agent with `mode: serverless` + `minReplicas: 0`.

### How do I report a bug?

[GitHub Issues](https://github.com/kryptonhq/runtime/issues).
