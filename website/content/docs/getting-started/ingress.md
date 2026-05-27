---
title: Bring your own ingress
weight: 5
description: Front Krypton with Gateway API, Nginx, or any L7 ingress — plus webhooks and non-Kubernetes notes.
---

The `krypton-gateway` Service exposes plain HTTP on port `8080`.
Production installs put their own L7 ingress in front for TLS
termination, auth, and rate limiting. The path prefix you route is
always `/v1/agents` — the gateway handles everything under it (see
[Ports & endpoints](../first-agent/#ports--endpoints--the-two-minute-mental-model)).

Whichever ingress you pick, two settings are non-negotiable:

- **Disable response buffering** — SSE / chunked HTTP need to flush
  as the agent emits, not at EOF.
- **Bump the read timeout** above your agents' worst-case response
  time (default 60s on most ingresses is too low for an LLM-backed
  agent).

## Gateway API

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: krypton-public
  namespace: krypton-system
spec:
  parentRefs:
    - name: my-gateway
  hostnames: ["agents.example.com"]
  rules:
    - matches:
        - path: { type: PathPrefix, value: /v1/agents }
      backendRefs:
        - name: krypton-gateway
          port: 8080
```

## Nginx Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: krypton-public
  namespace: krypton-system
  annotations:
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "300"
spec:
  ingressClassName: nginx
  rules:
    - host: agents.example.com
      http:
        paths:
          - path: /v1/agents
            pathType: Prefix
            backend:
              service:
                name: krypton-gateway
                port: { number: 8080 }
```

## Webhooks (optional)

Validating + defaulting webhooks are off by default because they
require TLS plumbing (cert-manager or hand-minted certs). The CRD's
OpenAPI validation catches most spec mistakes either way.

To enable:

```yaml
manager:
  enableWebhooks: true
```

Then plumb a serving cert into the manager. [cert-manager][cm] is the
lowest-friction path.

[cm]: https://cert-manager.io/

## Operating outside Kubernetes

There isn't a non-Kubernetes path. Krypton's design hard-relies on the
Kubernetes API for desired-state, scaling, and pod scheduling.
