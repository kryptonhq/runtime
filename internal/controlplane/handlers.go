/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package controlplane implements the Krypton control plane HTTP API.
//
// The control plane serves agent registry and status queries from an
// informer-backed cache. Persistent storage (Postgres) is deferred to a
// later milestone — see DESIGN.md §6 — since the data it would hold
// (invocations, aggregated metrics) doesn't exist until M6/M8. CRDs remain
// the source of truth.
package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/metrics"
)

// AgentView is the public JSON representation of an Agent. It hides
// Kubernetes plumbing (managedFields, finalizers, ownerReferences) that
// API consumers don't need.
type AgentView struct {
	Name      string                      `json:"name"`
	Namespace string                      `json:"namespace"`
	UID       string                      `json:"uid,omitempty"`
	Spec      kryptonv1alpha1.AgentSpec   `json:"spec"`
	Status    kryptonv1alpha1.AgentStatus `json:"status"`
}

// ModelView is the public JSON representation of a Model.
type ModelView struct {
	Name      string                      `json:"name"`
	Namespace string                      `json:"namespace"`
	UID       string                      `json:"uid,omitempty"`
	Spec      kryptonv1alpha1.ModelSpec   `json:"spec"`
	Status    kryptonv1alpha1.ModelStatus `json:"status"`
}

// API holds dependencies for the REST handlers. It is bound to a
// controller-runtime client whose underlying cache must already be
// started.
type API struct {
	Client client.Client
}

// Handler builds the http.Handler tree for the public REST surface plus
// /healthz and /readyz. Prometheus /metrics is served by the manager on
// its own bind address.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/agents", observe("list_agents", http.HandlerFunc(a.listAgents)))
	mux.Handle("GET /v1/agents/{namespace}/{name}", observe("get_agent", http.HandlerFunc(a.getAgent)))
	mux.Handle("GET /v1/agents/{namespace}/{name}/status", observe("get_agent_status", http.HandlerFunc(a.getAgentStatus)))
	mux.Handle("GET /v1/models", observe("list_models", http.HandlerFunc(a.listModels)))
	mux.Handle("GET /v1/models/{namespace}/{name}", observe("get_model", http.HandlerFunc(a.getModel)))
	mux.Handle("GET /v1/models/{namespace}/{name}/status", observe("get_model_status", http.HandlerFunc(a.getModelStatus)))
	a.registerMCPRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// Operator UI lives under /ui/* and falls back to index.html for
	// SPA routing. Root path redirects to /ui/ for convenience.
	mux.Handle("/ui/", UI())
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})
	return mux
}

// observe wraps a handler with request count + duration metrics. The
// route label is a fixed template ("list_agents") rather than the raw URL
// so cardinality stays bounded.
func observe(route string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &codeRecorder{ResponseWriter: w, code: http.StatusOK}
		h.ServeHTTP(rec, r)
		metrics.APIRequestsTotal.WithLabelValues(route, r.Method, strconv.Itoa(rec.code)).Inc()
		metrics.APIRequestDurationSeconds.WithLabelValues(route).Observe(time.Since(start).Seconds())
	})
}

type codeRecorder struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (c *codeRecorder) WriteHeader(code int) {
	if !c.wroteHeader {
		c.code = code
		c.wroteHeader = true
	}
	c.ResponseWriter.WriteHeader(code)
}

// listAgents supports server-side filtering, sorting, and pagination via
// query params:
//
//	?namespace=<ns>     restrict to one namespace
//	?protocol=<p>       restrict by protocol (a2a | mcp | http)
//	?q=<text>           case-insensitive substring match on name+namespace+image
//	?sort=<field>       one of name | namespace | phase | replicas | image
//	?order=asc|desc     default asc
//	?page=<int>         1-based; default 1
//	?pageSize=<int>     clamped to [1, 100]; default 20
//
// Response: {items, page, pageSize, total}.
func (a *API) listAgents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var list kryptonv1alpha1.AgentList
	opts := []client.ListOption{}
	if ns := q.Get("namespace"); ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := a.Client.List(r.Context(), &list, opts...); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("list agents: %w", err))
		return
	}

	// Filter
	protocol := strings.ToLower(strings.TrimSpace(q.Get("protocol")))
	search := strings.ToLower(strings.TrimSpace(q.Get("q")))
	views := make([]AgentView, 0, len(list.Items))
	for i := range list.Items {
		v := toView(&list.Items[i])
		if protocol != "" && string(v.Spec.Protocol) != protocol {
			continue
		}
		if search != "" && !matchesSearch(&v, search) {
			continue
		}
		views = append(views, v)
	}

	// Sort
	sortField := strings.ToLower(strings.TrimSpace(q.Get("sort")))
	if sortField == "" {
		sortField = "name"
	}
	order := strings.ToLower(strings.TrimSpace(q.Get("order")))
	desc := order == "desc"
	sort.SliceStable(views, func(i, j int) bool {
		return lessAgent(&views[i], &views[j], sortField, desc)
	})

	total := len(views)

	// Paginate
	page := atoiOr(q.Get("page"), 1)
	if page < 1 {
		page = 1
	}
	pageSize := atoiOr(q.Get("pageSize"), 20)
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pageItems := views[start:end]

	writeJSON(w, http.StatusOK, map[string]any{
		"items":    pageItems,
		"page":     page,
		"pageSize": pageSize,
		"total":    total,
	})
}

func matchesSearch(v *AgentView, needle string) bool {
	if strings.Contains(strings.ToLower(v.Name), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(v.Namespace), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(v.Spec.Image), needle) {
		return true
	}
	return false
}

func lessAgent(a, b *AgentView, field string, desc bool) bool {
	cmp := func(s1, s2 string) bool {
		if desc {
			return s1 > s2
		}
		return s1 < s2
	}
	cmpInt := func(n1, n2 int32) bool {
		if desc {
			return n1 > n2
		}
		return n1 < n2
	}
	switch field {
	case "namespace":
		if a.Namespace != b.Namespace {
			return cmp(a.Namespace, b.Namespace)
		}
		return cmp(a.Name, b.Name)
	case "phase":
		ap, bp := string(a.Status.Phase), string(b.Status.Phase)
		if ap != bp {
			return cmp(ap, bp)
		}
		return cmp(a.Name, b.Name)
	case "replicas":
		if a.Status.Replicas != b.Status.Replicas {
			return cmpInt(a.Status.Replicas, b.Status.Replicas)
		}
		return cmp(a.Name, b.Name)
	case "image":
		if a.Spec.Image != b.Spec.Image {
			return cmp(a.Spec.Image, b.Spec.Image)
		}
		return cmp(a.Name, b.Name)
	default: // "name"
		if a.Name != b.Name {
			return cmp(a.Name, b.Name)
		}
		return cmp(a.Namespace, b.Namespace)
	}
}

func atoiOr(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func (a *API) getAgent(w http.ResponseWriter, r *http.Request) {
	agent, err := a.fetch(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toView(agent))
}

func (a *API) getAgentStatus(w http.ResponseWriter, r *http.Request) {
	agent, err := a.fetch(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, agent.Status)
}

// listModels supports the same query shape as listAgents, with model-specific
// sort/search fields:
//
//	?namespace=<ns>     restrict to one namespace
//	?q=<text>           case-insensitive match on name+namespace+source+runtime
//	?sort=<field>       one of name | namespace | phase | replicas | runtime | source
//	?order=asc|desc     default asc
//	?page=<int>         1-based; default 1
//	?pageSize=<int>     clamped to [1, 100]; default 20
func (a *API) listModels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var list kryptonv1alpha1.ModelList
	opts := []client.ListOption{}
	if ns := q.Get("namespace"); ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := a.Client.List(r.Context(), &list, opts...); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("list models: %w", err))
		return
	}

	search := strings.ToLower(strings.TrimSpace(q.Get("q")))
	views := make([]ModelView, 0, len(list.Items))
	for i := range list.Items {
		v := toModelView(&list.Items[i])
		if search != "" && !matchesModelSearch(&v, search) {
			continue
		}
		views = append(views, v)
	}

	sortField := strings.ToLower(strings.TrimSpace(q.Get("sort")))
	if sortField == "" {
		sortField = "name"
	}
	order := strings.ToLower(strings.TrimSpace(q.Get("order")))
	desc := order == "desc"
	sort.SliceStable(views, func(i, j int) bool {
		return lessModel(&views[i], &views[j], sortField, desc)
	})

	total := len(views)
	page := atoiOr(q.Get("page"), 1)
	if page < 1 {
		page = 1
	}
	pageSize := atoiOr(q.Get("pageSize"), 20)
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":    views[start:end],
		"page":     page,
		"pageSize": pageSize,
		"total":    total,
	})
}

func matchesModelSearch(v *ModelView, needle string) bool {
	if strings.Contains(strings.ToLower(v.Name), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(v.Namespace), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(v.Spec.Source.HuggingFace), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(v.Spec.Source.File), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(string(v.Spec.Runtime)), needle) {
		return true
	}
	return false
}

func lessModel(a, b *ModelView, field string, desc bool) bool {
	cmp := func(s1, s2 string) bool {
		if desc {
			return s1 > s2
		}
		return s1 < s2
	}
	cmpInt := func(n1, n2 int32) bool {
		if desc {
			return n1 > n2
		}
		return n1 < n2
	}
	switch field {
	case "namespace":
		if a.Namespace != b.Namespace {
			return cmp(a.Namespace, b.Namespace)
		}
		return cmp(a.Name, b.Name)
	case "phase":
		ap, bp := string(a.Status.Phase), string(b.Status.Phase)
		if ap != bp {
			return cmp(ap, bp)
		}
		return cmp(a.Name, b.Name)
	case "replicas":
		if a.Status.Replicas != b.Status.Replicas {
			return cmpInt(a.Status.Replicas, b.Status.Replicas)
		}
		return cmp(a.Name, b.Name)
	case "runtime":
		ar, br := string(a.Spec.Runtime), string(b.Spec.Runtime)
		if ar != br {
			return cmp(ar, br)
		}
		return cmp(a.Name, b.Name)
	case "source":
		as, bs := a.Spec.Source.HuggingFace, b.Spec.Source.HuggingFace
		if as != bs {
			return cmp(as, bs)
		}
		return cmp(a.Name, b.Name)
	default:
		if a.Name != b.Name {
			return cmp(a.Name, b.Name)
		}
		return cmp(a.Namespace, b.Namespace)
	}
}

func (a *API) getModel(w http.ResponseWriter, r *http.Request) {
	model, err := a.fetchModel(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toModelView(model))
}

func (a *API) getModelStatus(w http.ResponseWriter, r *http.Request) {
	model, err := a.fetchModel(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.Status)
}

func (a *API) fetch(ctx context.Context, ns, name string) (*kryptonv1alpha1.Agent, error) {
	if ns == "" || name == "" {
		return nil, errBadRequest("namespace and name are required")
	}
	var agent kryptonv1alpha1.Agent
	if err := a.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (a *API) fetchModel(ctx context.Context, ns, name string) (*kryptonv1alpha1.Model, error) {
	if ns == "" || name == "" {
		return nil, errBadRequest("namespace and name are required")
	}
	var model kryptonv1alpha1.Model
	if err := a.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &model); err != nil {
		return nil, err
	}
	return &model, nil
}

func toView(a *kryptonv1alpha1.Agent) AgentView {
	return AgentView{
		Name:      a.Name,
		Namespace: a.Namespace,
		UID:       string(a.UID),
		Spec:      a.Spec,
		Status:    a.Status,
	}
}

func toModelView(m *kryptonv1alpha1.Model) ModelView {
	return ModelView{
		Name:      m.Name,
		Namespace: m.Namespace,
		UID:       string(m.UID),
		Spec:      m.Spec,
		Status:    m.Status,
	}
}

// --- errors ----------------------------------------------------------------

type apiError struct {
	status int
	msg    string
}

func (e apiError) Error() string { return e.msg }

func errBadRequest(msg string) error { return apiError{status: http.StatusBadRequest, msg: msg} }

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func writeAPIErr(w http.ResponseWriter, err error) {
	var ae apiError
	switch {
	case apierrors.IsNotFound(err):
		writeErr(w, http.StatusNotFound, err)
	case errors.As(err, &ae):
		writeErr(w, ae.status, err)
	default:
		writeErr(w, http.StatusInternalServerError, err)
	}
}
