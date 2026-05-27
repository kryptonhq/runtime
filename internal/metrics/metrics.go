/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package metrics defines Krypton's Prometheus metrics. All metrics
// register against controller-runtime's metrics registry — the same one
// the manager exposes on --metrics-bind-address — so a single scrape
// target per component picks them up alongside the runtime's own series.
//
// The sidecar (krypton-proxy) ships its own /metrics on a different port
// because it runs without a controller-runtime manager. See
// internal/sidecar.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Gateway metrics. Buckets target the 100ms–5s P50/P95 range typical for
// agent invocations.
var (
	InvocationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "krypton_invocations_total",
			Help: "Invocation requests served by the gateway.",
		},
		[]string{"agent", "namespace", "status"},
	)

	InvocationDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "krypton_invocation_duration_seconds",
			Help:    "End-to-end gateway invocation latency in seconds.",
			Buckets: []float64{0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"agent", "namespace"},
	)

	ColdStartsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "krypton_cold_starts_total",
			Help: "Invocations that triggered a cold start (zero ready endpoints at arrival).",
		},
		[]string{"agent", "namespace"},
	)

	BufferDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "krypton_buffer_depth",
			Help: "Cold-start requests currently buffered, per agent.",
		},
		[]string{"agent", "namespace"},
	)
)

// Scaler metrics.
var (
	ScalerDecisionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "krypton_scaler_decisions_total",
			Help: "Scaling decisions broken down by direction (up | down | noop).",
		},
		[]string{"agent", "namespace", "direction"},
	)

	AgentReplicasDesired = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "krypton_agent_replicas_desired",
			Help: "Current desiredReplicas the scaler has written for each agent.",
		},
		[]string{"agent", "namespace"},
	)
)

// Control plane metrics.
var (
	APIRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "krypton_api_requests_total",
			Help: "Control plane REST requests, labelled by route template and HTTP status.",
		},
		[]string{"route", "method", "code"},
	)

	APIRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "krypton_api_request_duration_seconds",
			Help:    "Control plane request latency.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		InvocationsTotal,
		InvocationDurationSeconds,
		ColdStartsTotal,
		BufferDepth,
		ScalerDecisionsTotal,
		AgentReplicasDesired,
		APIRequestsTotal,
		APIRequestDurationSeconds,
	)
}
