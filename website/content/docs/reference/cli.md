---
title: CLI flags
weight: 2
description: Every flag per binary.
---

Every binary uses Go's standard `flag` package. `--help` on any of them
prints the full list.

## Manager (`cmd/manager`)

The controller + scaler.

| Flag                            | Default                                | Notes                                              |
| ------------------------------- | -------------------------------------- | -------------------------------------------------- |
| `--metrics-bind-address`        | `:8080`                                | Prometheus scrape target                           |
| `--health-probe-bind-address`   | `:8081`                                | `/healthz`, `/readyz`                              |
| `--leader-elect`                | `false`                                | Required when running multiple replicas            |
| `--enable-webhooks`             | `true`                                 | Set false to skip webhook server (Helm chart does) |
| `--proxy-image`                 | `ghcr.io/kryptonhq/krypton-proxy:latest` | Sidecar image injected into agent pods           |
| `--enable-scaler`               | `true`                                 | Disable to run controller only                     |
| `--scaler-interval-ms`          | `1000`                                 | Tick interval for the scaling decider              |
| `--scaler-stable-window-ms`     | `60000`                                | Hysteresis: refuse scale-down within this window   |
| `--sidecar-port`                | `8888`                                 | Where the scaler probes `/_krypton/inflight`       |

## Control plane (`cmd/control-plane`)

REST API + UI.

| Flag                            | Default            | Notes                                                  |
| ------------------------------- | ------------------ | ------------------------------------------------------ |
| `--api-bind-address`            | `:8090`            | Public REST API + UI                                   |
| `--metrics-bind-address`        | `:8091`            | Prometheus                                             |
| `--health-probe-bind-address`   | `:8092`            | controller-runtime probes                              |
| `--database-url`                | `$DATABASE_URL`    | Postgres DSN; empty = in-memory store                  |

## Gateway (`cmd/gateway`)

Public ingress + activator.

| Flag                              | Default | Notes                                                                 |
| --------------------------------- | ------- | --------------------------------------------------------------------- |
| `--listen-address`                | `:8080` | Public HTTP                                                           |
| `--metrics-bind-address`          | `:8081` | Prometheus                                                            |
| `--health-probe-bind-address`     | `:8082` | controller-runtime probes                                             |
| `--max-buffer-per-agent`          | `100`   | Cold-start waiters per agent before 503                               |
| `--poll-interval-ms`              | `50`    | Endpoint-readiness poll interval                                      |
| `--default-startup-timeout-ms`    | `30000` | Used when `spec.startupTimeout` is unset                              |

## Sidecar (`cmd/krypton-proxy`)

Config via environment variables (the controller injects these at pod
template render time):

| Env var                       | Default                        | Notes                                                  |
| ----------------------------- | ------------------------------ | ------------------------------------------------------ |
| `KRYPTON_AGENT_NAME`          | `unknown`                      | Metric label                                           |
| `KRYPTON_AGENT_NAMESPACE`     | `default`                      | Metric label                                           |
| `KRYPTON_LISTEN_ADDR`         | `:8888`                        | Sidecar port (Service `TargetPort`)                   |
| `KRYPTON_UPSTREAM_URL`        | `http://127.0.0.1:8080`        | User container address                                 |
| `KRYPTON_CONCURRENCY`         | `8`                            | In-flight cap; over → 503 + `Retry-After`              |
| `KRYPTON_MODE`                | `serverless`                   | `serverless` or `always-on`                            |
| `KRYPTON_IDLE_TIMEOUT`        | `300s`                         | Idle threshold (informational; scaler handles scale-to-zero) |
| `KRYPTON_SHUTDOWN_TIMEOUT`    | `25s`                          | Graceful drain bound on SIGTERM                        |
