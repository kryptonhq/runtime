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
        Kubernetes-native serving for AI agents, self-hosted LLMs, and MCP
        servers. Deploy containers or Hugging Face GGUF models with CRDs,
        then reach them through stable HTTP and OpenAI-compatible endpoints.
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

<span class="kr-term__comment"># 2. Deploy a no-secrets agent</span>
<span class="kr-term__prompt">$</span> <span class="kr-term__cmd">kubectl</span> apply -f https://raw.githubusercontent.com\
    /kryptonhq/runtime/main/examples/agent/python/helloworld/agent.yaml

<span class="kr-term__comment"># 3. Serve Qwen2.5 from Hugging Face</span>
<span class="kr-term__prompt">$</span> <span class="kr-term__cmd">kubectl</span> create namespace models
<span class="kr-term__prompt">$</span> <span class="kr-term__cmd">kubectl</span> apply -f https://raw.githubusercontent.com\
    /kryptonhq/runtime/main/config/samples/llm/qwen2.5-0.5b.yaml

<span class="kr-term__comment"># 4. Call the model with the OpenAI API shape</span>
<span class="kr-term__prompt">$</span> <span class="kr-term__cmd">curl</span> http://localhost:8080/v1/chat/completions \
    -d '{"model":"qwen2-0-5b","messages":[{"role":"user","content":"Hi"}]}'
</code></pre>
      </div>
    </div>
  </div>
</div>
{{< /blocks/cover >}}

{{% blocks/section color="primary" type="row" %}}

{{% blocks/feature icon="fa-rocket" title="Agents as cluster resources" %}}
A single `Agent` custom resource registers your A2A, plain HTTP, or
framework-backed container. Krypton handles lifecycle, routing, scaling
signals, and operator visibility.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-brain" title="Self-host LLMs on Kubernetes" %}}
A `Model` custom resource names a Hugging Face GGUF file and runs it
with llama.cpp in your cluster. Serve local models with Kubernetes-native
lifecycle, resources, and observability.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-cube" title="MCP, first-class" %}}
Run any HTTP-transport MCP server as an `Agent`, or wrap a stdio MCP
binary in the bundled bridge. The operator UI introspects each server's
tools.
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

{{% blocks/feature icon="fa-stream" title="Streaming-native" %}}
SSE, chunked HTTP, and WebSocket upgrades pass through the gateway with
immediate flushing. Chat completions can stream without buffering away
the model's first token.
{{% /blocks/feature %}}

{{% /blocks/section %}}

{{% blocks/section color="primary" type="row" %}}

{{% blocks/feature icon="fa-gauge-high" title="Concurrency-aware agents" %}}
For agent workloads, the per-pod sidecar enforces in-flight caps and
surfaces live load. Replicas can keep up with traffic without exceeding
the configured per-pod ceiling.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-shuffle" title="OpenAI-compatible serving" %}}
Each self-hosted `Model` is reachable through familiar OpenAI API paths
like `/v1/models` and `/v1/chat/completions`, so existing SDKs can call
your in-cluster llama.cpp pods.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-microchip" title="llama.cpp built in" %}}
Start with GGUF models from Hugging Face. Krypton creates the Deployment
and Service, passes the right llama.cpp flags, and tracks model readiness
in Kubernetes status.
{{% /blocks/feature %}}

{{% /blocks/section %}}
