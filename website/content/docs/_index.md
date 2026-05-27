---
title: Documentation
linkTitle: Documentation
weight: 20
---

Krypton is a Kubernetes-native runtime orchestration platform for AI
agents and MCP servers. Deploy an A2A-compatible container with a
single CRD; the runtime handles scaling, routing, and lifecycle.

{{% alert title="MVP scope" color="info" %}}
The MVP runs agents in **always-on mode**. The serverless / scale-to-zero
code path is implemented and exercised by tests, but isn't recommended
for use yet — see [Serverless mode (paused)](/docs/architecture/components/#serverless-mode-paused)
for the current status.
{{% /alert %}}

## Start here

- **[Local testing on kind](/docs/getting-started/local-testing/)** —
  full cluster up in five minutes
- **[Your first agent](/docs/getting-started/first-agent/)** — beyond
  the bundled echo sample
- **[Architecture overview](/docs/architecture/overview/)** — what's
  running and why

## Pre-alpha

APIs are unstable. See the
[design document](https://github.com/kryptonhq/runtime/blob/main/DESIGN.md)
for the full technical design and
[`progress.md`](https://github.com/kryptonhq/runtime/blob/main/progress.md)
for milestone tracking.
