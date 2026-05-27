---
title: Krypton Runtime
linkTitle: Krypton
---

{{< blocks/cover title="" image_anchor="top" height="auto" color="dark" >}}
<div class="container py-5">
  <div class="row align-items-center g-5">
    <div class="col-lg-6 text-start">
      <span class="badge bg-light text-dark mb-3" style="font-size: 0.75rem; letter-spacing: 0.05em;">
        {{< version >}}
      </span>
      <h1 class="display-5 fw-bold mb-3">Krypton Runtime</h1>
      <p class="lead mb-4">
        Kubernetes-native runtime orchestration for AI agents and MCP servers.
        Deploy any A2A-compatible container with a single CRD; the runtime
        handles routing, scaling, and lifecycle.
      </p>
      <div class="d-flex flex-wrap gap-2">
        <a class="btn btn-primary" href="/docs/getting-started/installation/">
          Get started <i class="fas fa-arrow-right ms-1"></i>
        </a>
        <a class="btn btn-outline-light" href="/docs/">
          Read the docs
        </a>
        <a class="btn btn-outline-light" href="https://github.com/kryptonhq/runtime">
          <i class="fab fa-github me-1"></i> GitHub
        </a>
      </div>
    </div>
    <div class="col-lg-6">
      <div class="kr-term shadow-lg">
        <div class="kr-term__bar">
          <span class="kr-term__dot kr-term__dot--red"></span>
          <span class="kr-term__dot kr-term__dot--amber"></span>
          <span class="kr-term__dot kr-term__dot--green"></span>
          <span class="kr-term__title">krypton ~ zsh</span>
        </div>
        <pre class="kr-term__body"><code><span class="kr-term__comment"># 1. Install Krypton with Helm</span>
<span class="kr-term__prompt">$</span> <span class="kr-term__cmd">helm</span> install krypton oci://ghcr.io/kryptonhq/charts/krypton \
    --namespace krypton-system --create-namespace

<span class="kr-term__comment"># 2. Deploy helloworld agent</span>
<span class="kr-term__prompt">$</span> <span class="kr-term__cmd">kubectl</span> apply -f https://raw.githubusercontent.com\
    /kryptonhq/runtime/main/examples/agent/python/helloworld/agent.yaml
</code></pre>
      </div>
    </div>
  </div>
</div>
{{< /blocks/cover >}}

{{% blocks/section color="primary" type="row" %}}

{{% blocks/feature icon="fa-rocket" title="One CRD, any agent" %}}
A single `Agent` custom resource registers your container image with the
runtime. Routing, scaling, and pod lifecycle are handled — you just
bring the HTTP handler.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-gauge-high" title="Concurrency-aware" %}}
The per-pod sidecar enforces in-flight caps and surfaces live load.
Replicas auto-scale to keep up with traffic without exceeding the
configured per-pod ceiling.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-stream" title="Streaming-native" %}}
SSE, chunked HTTP, and WebSocket upgrades pass through the gateway with
immediate flushing. No buffering, no SSE tax.
{{% /blocks/feature %}}

{{% /blocks/section %}}

{{% blocks/section color="white" type="row" %}}

{{% blocks/feature icon="fa-chart-line" title="Prometheus-native observability" %}}
Every component exposes `krypton_*` series — invocations, latency,
desired replicas, scaler decisions, sidecar in-flight. A starter
Grafana dashboard ships in the repo.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-shield-halved" title="BYO ingress" %}}
The gateway ships as a ClusterIP. Put your existing ingress
(Envoy / Nginx / ALB / Cloudflare) in front for TLS, auth, rate
limiting — Krypton doesn't reinvent any of it.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-cube" title="MCP, first-class" %}}
Run any HTTP-transport MCP server as an `Agent`, or wrap a stdio MCP
binary in the bundled bridge. The operator UI introspects each
server's tools.
{{% /blocks/feature %}}

{{% /blocks/section %}}
