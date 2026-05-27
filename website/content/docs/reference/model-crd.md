---
title: Model CRD
weight: 2
description: Every field on the Model spec.
---

`apiVersion: krypton.ai/v1alpha1`, `kind: Model`, namespaced
(short name `mdl`).

The `Model` CRD declares "host this Hugging Face GGUF as an OpenAI-compatible
model named *X*". The controller turns it into a Deployment running
[llama.cpp][llamacpp]'s HTTP server, plus a Service. The gateway
aggregates every `Model` in the cluster at `/v1/models` and routes
incoming `/v1/chat/completions` (etc.) requests by the `model` field
in the body.

## Minimal example

```yaml
apiVersion: krypton.ai/v1alpha1
kind: Model
metadata:
  name: qwen2-0-5b
  namespace: models
spec:
  source:
    huggingface: Qwen/Qwen2.5-0.5B-Instruct-GGUF
    file: qwen2.5-0.5b-instruct-q4_k_m.gguf
```

That's the smallest valid `Model` — every other field has a default.

## Full example

```yaml
apiVersion: krypton.ai/v1alpha1
kind: Model
metadata:
  name: qwen2-0-5b
  namespace: models
spec:
  # Weights
  source:
    huggingface: Qwen/Qwen2.5-0.5B-Instruct-GGUF
    file:        qwen2.5-0.5b-instruct-q4_k_m.gguf

  # Runtime
  runtime: llama.cpp           # only value supported today
  image: ""                    # blank = built-in llama.cpp:server image
  imagePullPolicy: IfNotPresent
  imagePullSecrets: []

  # Networking
  port: 8080

  # Extra args appended to llama-server
  args:
    - "--ctx-size"
    - "4096"

  # Scaling (always-on; scale-to-zero on the roadmap)
  minReplicas: 1
  maxReplicas: 1

  # Pod
  resources:
    requests: { cpu: 500m, memory: 1Gi }
    limits:   { cpu: "2",   memory: 2Gi }
  env: []
  envFrom:
    - secretRef: { name: hf-token }   # for gated repos: HUGGINGFACE_HUB_TOKEN
  serviceAccountName: ""              # blank = auto-create

  # Lifecycle
  startupTimeout: 600s
```

## Spec reference

### Source

| Field                  | Type              | Default | Notes                                                          |
| ---------------------- | ----------------- | ------- | -------------------------------------------------------------- |
| `source.huggingface`   | string (required) | —       | Hugging Face repo id, e.g. `Qwen/Qwen2.5-0.5B-Instruct-GGUF`   |
| `source.file`          | string (required) | —       | GGUF file within the repo to load                              |

Gated repos require a token. Mount it as `HUGGINGFACE_HUB_TOKEN` via
`envFrom`.

### Runtime

| Field             | Type                          | Default                              | Notes                                                              |
| ----------------- | ----------------------------- | ------------------------------------ | ------------------------------------------------------------------ |
| `runtime`         | enum                          | `llama.cpp`                          | Only `llama.cpp` is supported today                                |
| `image`           | string                        | `ghcr.io/ggml-org/llama.cpp:server`  | Override the runtime container image                              |
| `imagePullPolicy` | string                        | `IfNotPresent`                       | Standard K8s pull policy                                          |
| `imagePullSecrets`| `[]LocalObjectReference`      | —                                    | Same shape as a pod's `imagePullSecrets`                          |

### Networking

| Field   | Type            | Default | Notes                                |
| ------- | --------------- | ------- | ------------------------------------ |
| `port`  | int32 (1–65535) | `8080`  | Port `llama-server` listens on       |

### Args

`args` is appended to the runtime entrypoint **after** the
controller-owned flags (`--host`, `--port`, `--hf-repo`, `--hf-file`,
`--alias`). Use it for tuning knobs like `--ctx-size`, `--n-gpu-layers`,
`--parallel`. Don't redefine the controller-owned flags from here.

### Scaling

| Field         | Type        | Default | Notes                                              |
| ------------- | ----------- | ------- | -------------------------------------------------- |
| `minReplicas` | int32 (≥ 1) | `1`     | Models are always-on; zero floor is not supported |
| `maxReplicas` | int32 (≥ 1) | `1`     | Caps replicas; future autoscaler will honor this  |

### Pod

| Field                | Type                          | Default | Notes                                                |
| -------------------- | ----------------------------- | ------- | ---------------------------------------------------- |
| `resources`          | `corev1.ResourceRequirements` | —       | Standard pod resource block                          |
| `env`                | `[]corev1.EnvVar`             | —       | Passed to the runtime container                      |
| `envFrom`            | `[]corev1.EnvFromSource`      | —       | Use for `HUGGINGFACE_HUB_TOKEN` on gated repos       |
| `serviceAccountName` | string                        | `""`    | Empty = auto-created SA with minimal permissions     |

### Lifecycle

| Field            | Type     | Default | Notes                                                                  |
| ---------------- | -------- | ------- | ---------------------------------------------------------------------- |
| `startupTimeout` | duration | `600s`  | Cold-pull grace. First start downloads weights from HF — keep generous |

## Status (read-only)

| Field                | Type      | Written by  |
| -------------------- | --------- | ----------- |
| `phase`              | enum      | Manager     |
| `replicas`           | int32     | Manager     |
| `readyReplicas`      | int32     | Manager     |
| `url`                | string    | Manager     |
| `observedGeneration` | int64     | Manager     |
| `conditions`         | `[]Cond.` | Manager     |

### Phases

| Phase     | Meaning                                      |
| --------- | -------------------------------------------- |
| `Pending` | Pod exists but not yet ready (likely pulling weights) |
| `Ready`   | At least one replica reports ready           |
| `Failed`  | Persistent reconcile errors (e.g. crashloop) |

`status.url` points at the in-cluster OpenAI base URL for the pod,
e.g. `http://qwen2-0-5b.models.svc:8080/v1`. Most callers go through
the gateway instead.

## OpenAI compatibility

Once `Ready`, the gateway exposes the model under these standard paths:

| Method | Path                          | Notes                                             |
| ------ | ----------------------------- | ------------------------------------------------- |
| `GET`  | `/v1/models`                  | Lists every `Model` in the cluster                |
| `GET`  | `/v1/models/{id}`             | OpenAI model card for one `Model` by name         |
| `POST` | `/v1/chat/completions`        | Routes by body `model`                            |
| `POST` | `/v1/completions`             | Routes by body `model`                            |
| `POST` | `/v1/embeddings`              | Routes by body `model`                            |

The OpenAI-facing `id` is `metadata.name`. If two `Model` CRs share a
name across namespaces, the gateway picks one and logs the collision —
keep model names unique.

[llamacpp]: https://github.com/ggerganov/llama.cpp
