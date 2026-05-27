/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package sidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Internal paths reserved for the sidecar. Anything else is reverse-proxied
// to the upstream user container.
const (
	pathHealthz  = "/healthz"
	pathReadyz   = "/readyz"
	pathMetrics  = "/metrics"
	pathInflight = "/_krypton/inflight"
)

// Proxy is the sidecar's HTTP handler. It enforces concurrency, exposes
// health/metrics/inflight endpoints, and reverse-proxies everything else.
type Proxy struct {
	cfg     Config
	sem     chan struct{}
	reverse *httputil.ReverseProxy

	inflight       atomic.Int64
	lastActivityNs atomic.Int64
	shuttingDown   atomic.Bool

	// metrics
	requestsTotal *prometheus.CounterVec
	inflightGauge prometheus.Gauge
	rejectedTotal *prometheus.CounterVec
	registry      *prometheus.Registry
}

// NewProxy constructs a Proxy from config. It returns an error if the
// upstream URL doesn't parse — fail fast at boot rather than at first
// request.
func NewProxy(cfg Config) (*Proxy, error) {
	u, err := url.Parse(cfg.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream url: %w", err)
	}
	p := &Proxy{
		cfg:      cfg,
		sem:      make(chan struct{}, cfg.Concurrency),
		registry: prometheus.NewRegistry(),
	}
	p.reverse = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = u.Scheme
			req.URL.Host = u.Host
			req.Host = u.Host
		},
		FlushInterval: -1, // immediate flush for streaming
	}
	p.lastActivityNs.Store(time.Now().UnixNano())

	labels := prometheus.Labels{"agent": cfg.AgentName, "namespace": cfg.AgentNamespace}
	p.requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "krypton_proxy_requests_total", Help: "Requests forwarded to the agent container."},
		[]string{"agent", "namespace", "code"},
	)
	p.rejectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "krypton_proxy_rejected_total", Help: "Requests rejected by the concurrency limiter."},
		[]string{"agent", "namespace", "reason"},
	)
	p.inflightGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "krypton_proxy_inflight", Help: "Currently in-flight requests.",
		ConstLabels: labels,
	})
	p.registry.MustRegister(p.requestsTotal, p.rejectedTotal, p.inflightGauge)
	return p, nil
}

// Handler returns an http.Handler with all sidecar routes wired up.
func (p *Proxy) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(pathHealthz, p.handleHealth)
	mux.HandleFunc(pathReadyz, p.handleReady)
	mux.Handle(pathMetrics, promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc(pathInflight, p.handleInflight)
	mux.HandleFunc("/", p.handleProxy)
	return mux
}

func (p *Proxy) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (p *Proxy) handleReady(w http.ResponseWriter, _ *http.Request) {
	if p.shuttingDown.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (p *Proxy) handleInflight(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"inflight":     p.inflight.Load(),
		"lastActivity": p.lastActivityNs.Load(),
		"concurrency":  p.cfg.Concurrency,
	})
}

func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	if p.shuttingDown.Load() {
		p.rejectedTotal.WithLabelValues(p.cfg.AgentName, p.cfg.AgentNamespace, "shutting_down").Inc()
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}

	// Non-blocking semaphore acquire — over the cap immediately returns 503
	// with Retry-After. The activator/gateway is responsible for backoff.
	select {
	case p.sem <- struct{}{}:
	default:
		p.rejectedTotal.WithLabelValues(p.cfg.AgentName, p.cfg.AgentNamespace, "over_capacity").Inc()
		w.Header().Set("Retry-After", "1")
		http.Error(w, "concurrency limit reached", http.StatusServiceUnavailable)
		return
	}
	defer func() { <-p.sem }()

	p.inflight.Add(1)
	p.inflightGauge.Inc()
	defer func() {
		p.inflight.Add(-1)
		p.inflightGauge.Dec()
		p.lastActivityNs.Store(time.Now().UnixNano())
	}()
	p.lastActivityNs.Store(time.Now().UnixNano())

	rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
	p.reverse.ServeHTTP(rec, r)
	p.requestsTotal.WithLabelValues(p.cfg.AgentName, p.cfg.AgentNamespace, codeLabel(rec.code)).Inc()
}

// IsIdle reports whether the proxy has been idle long enough to terminate
// the pod (serverless mode only). Always-on always returns false.
func (p *Proxy) IsIdle(now time.Time) bool {
	if p.cfg.Mode != ModeServerless {
		return false
	}
	if p.inflight.Load() > 0 {
		return false
	}
	last := time.Unix(0, p.lastActivityNs.Load())
	return now.Sub(last) >= p.cfg.IdleTimeout
}

// MarkShutdown causes /readyz to fail and new proxy requests to 503.
// Pre-existing in-flight requests continue draining via the semaphore.
func (p *Proxy) MarkShutdown() { p.shuttingDown.Store(true) }

// DrainAndShutdown blocks until in-flight requests complete or ctx fires.
// Callers should MarkShutdown first.
func (p *Proxy) DrainAndShutdown(ctx context.Context) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if p.inflight.Load() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// Inflight is exposed for the idle watcher and tests.
func (p *Proxy) Inflight() int64 { return p.inflight.Load() }

// codeLabel buckets HTTP status codes for metric cardinality.
func codeLabel(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

type statusRecorder struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.code = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Flush passes through http.Flusher so streaming responses still flush.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
