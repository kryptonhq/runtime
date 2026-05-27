/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kryptonhq/runtime/internal/metrics"
)

// agentPathPrefix is the URL prefix all agent traffic shares. The
// gateway strips /v1/agents/{namespace}/{name} and forwards everything
// after it to the pod verbatim — including /.well-known/*, OAuth
// callbacks, and the protocol-specific RPC endpoint at /.
const agentPathPrefix = "/v1/agents/"

// Gateway is the public HTTP entrypoint.
type Gateway struct {
	Activator *Activator

	// OverrideTarget is hooked in by tests to redirect outbound requests
	// from a real in-cluster DNS name to a local httptest server. nil
	// in production.
	OverrideTarget func(*url.URL) *url.URL
}

// Handler returns the gateway's HTTP handler tree.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/agents/", g.handleInvocation)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	return mux
}

// handleInvocation parses the URL, resolves the agent via the activator,
// and reverse-proxies the request to the resulting upstream.
//
// Routing:
//
//	/v1/agents/{namespace}/{name}[/...]
//
// Everything after the name is forwarded verbatim to the pod. The
// gateway makes no assumption about path shape — A2A's
// /.well-known/agent-card.json, MCP's JSON-RPC at /, OAuth callbacks
// at /oauth/callback, etc., all flow through this one handler.
func (g *Gateway) handleInvocation(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	key, rest, err := parseAgentPath(r.URL.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	logger := log.FromContext(ctx).WithValues("agent", key)

	target, _, err := g.Activator.Resolve(ctx, key)
	switch {
	case errors.Is(err, ErrBufferFull):
		w.Header().Set("Retry-After", "1")
		writeErr(w, http.StatusServiceUnavailable, err)
		recordInvocation(key, http.StatusServiceUnavailable, start)
		return
	case errors.Is(err, ErrColdStartTimeout):
		writeErr(w, http.StatusGatewayTimeout, err)
		recordInvocation(key, http.StatusGatewayTimeout, start)
		return
	case apierrors.IsNotFound(err):
		writeErr(w, http.StatusNotFound, fmt.Errorf("agent %s not found", key))
		recordInvocation(key, http.StatusNotFound, start)
		return
	case err != nil:
		logger.Error(err, "resolve agent")
		writeErr(w, http.StatusBadGateway, err)
		recordInvocation(key, http.StatusBadGateway, start)
		return
	}

	if g.OverrideTarget != nil {
		target = g.OverrideTarget(target)
	}

	// Rewrite the incoming path to the agent's invocation path. The agent
	// container sees `/<rest>` (without our /v1/agents/{ns}/{name}
	// prefix), so existing A2A/MCP handlers don't need to know about us.
	proxy := newReverseProxy(target)
	r2 := r.Clone(ctx)
	r2.URL.Path = rest
	r2.URL.RawPath = ""
	rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
	proxy.ServeHTTP(rec, r2)
	recordInvocation(key, rec.code, start)

	// Record the invocation asynchronously so the scaler can tell apart
	// idle-and-cold from idle-and-recently-active agents. Decoupled from
	// the request context so the patch survives the client disconnect.
	go g.Activator.RecordInvocation(context.WithoutCancel(ctx), key)
}

func recordInvocation(key types.NamespacedName, code int, start time.Time) {
	metrics.InvocationsTotal.WithLabelValues(key.Name, key.Namespace, strconv.Itoa(code)).Inc()
	metrics.InvocationDurationSeconds.WithLabelValues(key.Name, key.Namespace).Observe(time.Since(start).Seconds())
}

// statusRecorder captures the upstream's HTTP status for metric labeling
// without buffering the body — Flush passes through so streaming still
// works.
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

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// parseAgentPath extracts the (namespace, name) tuple and any remaining
// suffix from /v1/agents/{namespace}/{name}[/...].
//
// On bare /v1/agents/{ns}/{name} (with or without trailing slash) the
// returned rest is "/". On /v1/agents/{ns}/{name}/.well-known/foo it's
// "/.well-known/foo". The gateway makes no assumption about the suffix
// — it's forwarded to the pod verbatim.
func parseAgentPath(p string) (types.NamespacedName, string, error) {
	rem := strings.TrimPrefix(p, agentPathPrefix)
	if rem == p {
		return types.NamespacedName{}, "", fmt.Errorf("path must start with %s", agentPathPrefix)
	}
	parts := strings.SplitN(rem, "/", 3)
	if len(parts) < 2 {
		return types.NamespacedName{}, "", fmt.Errorf("expected /v1/agents/{namespace}/{name}[/...]")
	}
	ns, name := parts[0], parts[1]
	if ns == "" || name == "" {
		return types.NamespacedName{}, "", fmt.Errorf("namespace and name required")
	}
	rest := "/"
	if len(parts) == 3 && parts[2] != "" {
		rest = "/" + parts[2]
	}
	return types.NamespacedName{Namespace: ns, Name: name}, rest, nil
}

// newReverseProxy builds an httputil.ReverseProxy preconfigured for streaming
// and Host header rewriting.
func newReverseProxy(target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
			// Preserve client-facing trace context so the agent pod sees the
			// same traceparent the operator sent.
			if tp := pr.In.Header.Get("traceparent"); tp != "" {
				pr.Out.Header.Set("traceparent", tp)
			}
		},
		// FlushInterval = -1 forces immediate flushing — required for SSE
		// and chunked HTTP streaming so clients see tokens as they're
		// produced rather than at end-of-response.
		FlushInterval: -1,
	}
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
