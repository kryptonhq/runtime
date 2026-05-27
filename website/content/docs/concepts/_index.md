---
title: Concepts
weight: 3
description: The mental model for Krypton's resources, routing, and runtime components.
---

Start here when you want to understand how Krypton fits together before
changing production settings.

## Read in order

1. [Architecture](architecture/) explains the resource model, the four
   runtime components, and where state lives.
2. [Components](components/) goes deeper on the manager, control plane,
   gateway, sidecar, scaler, and model controller.
3. [Request lifecycle](request-lifecycle/) follows a request from the
   client through the gateway, sidecar, user container, scaler, and back.

## Core ideas

| Idea | Why it matters |
| ---- | -------------- |
| Kubernetes is the source of truth | `Agent` and `Model` resources describe desired state; controllers own Deployments, Services, and status. |
| Gateway traffic is separate from operator traffic | Clients call the gateway on `:8080`; the control plane UI and introspection APIs live on `:8090`. |
| Workloads stay ordinary containers | Krypton adds routing, lifecycle, scaling signals, and observability around your container without requiring a framework rewrite. |
| Models use OpenAI-compatible paths | Applications keep the familiar `/v1/models` and `/v1/chat/completions` API shape while operators manage in-cluster model pods. |
