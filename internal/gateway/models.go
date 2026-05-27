/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/metrics"
)

func recordModelInvocation(model string, code int, start time.Time) {
	metrics.ModelInvocationsTotal.WithLabelValues(model, strconv.Itoa(code)).Inc()
	metrics.ModelInvocationDurationSeconds.WithLabelValues(model).Observe(time.Since(start).Seconds())
}

// modelBodyByteLimit caps the request payload we'll buffer to peek at the
// `model` field. 1 MiB comfortably covers chat completions with very large
// system prompts; anything larger is rejected with 413.
const modelBodyByteLimit = 1 << 20

// ErrModelNotFound is returned when no Model CR matches the requested id.
var ErrModelNotFound = errors.New("model not found")

// ModelResolver looks up Model CRs cluster-wide by their OpenAI-facing id.
// The id is the Model resource name; if two CRs in different namespaces
// share a name, the resolver returns whichever the cache lists first and
// emits a debug log. Operators are expected to keep model names unique.
type ModelResolver struct {
	Client client.Client
}

// List returns every Model in the cluster. The slice is safe for the caller
// to iterate — it's a fresh copy from the controller-runtime cache.
func (m *ModelResolver) List(ctx context.Context) ([]kryptonv1alpha1.Model, error) {
	var list kryptonv1alpha1.ModelList
	if err := m.Client.List(ctx, &list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// Resolve finds the Model CR exposed under the given OpenAI id.
func (m *ModelResolver) Resolve(ctx context.Context, id string) (*kryptonv1alpha1.Model, error) {
	items, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	var matches []kryptonv1alpha1.Model
	for i := range items {
		if items[i].Name == id {
			matches = append(matches, items[i])
		}
	}
	if len(matches) == 0 {
		return nil, ErrModelNotFound
	}
	if len(matches) > 1 {
		log.FromContext(ctx).V(1).Info("model id collision across namespaces", "id", id, "count", len(matches))
	}
	return &matches[0], nil
}

// registerModelRoutes wires the OpenAI-compatible model endpoints onto the
// gateway mux. Routes follow OpenAI's path conventions so any SDK that
// targets `OPENAI_BASE_URL=https://krypton/v1` Just Works.
//
// GET  /v1/models                 — list every Model CR in the cluster
// GET  /v1/models/{id}            — retrieve one Model's OpenAI card
// POST /v1/chat/completions       — route by body.model
// POST /v1/completions            — route by body.model
// POST /v1/embeddings             — route by body.model
func (g *Gateway) registerModelRoutes(mux *http.ServeMux) {
	if g.ModelResolver == nil {
		return
	}
	mux.HandleFunc("GET /v1/models", g.handleListModels)
	mux.HandleFunc("GET /v1/models/{id}", g.handleGetModel)
	mux.HandleFunc("POST /v1/chat/completions", g.handleModelInvocation)
	mux.HandleFunc("POST /v1/completions", g.handleModelInvocation)
	mux.HandleFunc("POST /v1/embeddings", g.handleModelInvocation)
}

// openaiModel is OpenAI's `/v1/models` card shape. Krypton owns the id
// (== Model.metadata.name) and tags namespace/source for clients that want
// to disambiguate.
type openaiModel struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Created   int64  `json:"created"`
	OwnedBy   string `json:"owned_by"`
	Namespace string `json:"namespace,omitempty"`
	Source    string `json:"source,omitempty"`
}

type openaiModelList struct {
	Object string        `json:"object"`
	Data   []openaiModel `json:"data"`
}

func toOpenAIModel(m *kryptonv1alpha1.Model) openaiModel {
	src := ""
	if m.Spec.Source.HuggingFace != "" {
		src = "hf:" + m.Spec.Source.HuggingFace
		if m.Spec.Source.File != "" {
			src += "/" + m.Spec.Source.File
		}
	}
	return openaiModel{
		ID:        m.Name,
		Object:    "model",
		Created:   m.CreationTimestamp.Unix(),
		OwnedBy:   "krypton",
		Namespace: m.Namespace,
		Source:    src,
	}
}

func (g *Gateway) handleListModels(w http.ResponseWriter, r *http.Request) {
	items, err := g.ModelResolver.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("list models: %w", err))
		return
	}
	out := openaiModelList{Object: "list", Data: make([]openaiModel, 0, len(items))}
	for i := range items {
		out.Data = append(out.Data, toOpenAIModel(&items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (g *Gateway) handleGetModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, err := g.ModelResolver.Resolve(r.Context(), id)
	if errors.Is(err, ErrModelNotFound) {
		writeErr(w, http.StatusNotFound, fmt.Errorf("model %q not found", id))
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, toOpenAIModel(m))
}

// handleModelInvocation buffers the request body, peeks at the JSON
// `model` field, resolves it to a Model CR, and reverse-proxies to the
// in-cluster Service.
//
// Buffering the body up to modelBodyByteLimit is necessary because the
// OpenAI protocol places the model name in the request body. We restore
// the body on the cloned request so the upstream sees an identical payload.
// Streaming responses (SSE) flow back unaltered because the proxy uses
// FlushInterval = -1.
func (g *Gateway) handleModelInvocation(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Reject early on content-length when we can. Saves a body read.
	if r.ContentLength > modelBodyByteLimit {
		writeErr(w, http.StatusRequestEntityTooLarge, fmt.Errorf("request body exceeds %d bytes", modelBodyByteLimit))
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, modelBodyByteLimit+1))
	_ = r.Body.Close()
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("read body: %w", err))
		return
	}
	if len(body) > modelBodyByteLimit {
		writeErr(w, http.StatusRequestEntityTooLarge, fmt.Errorf("request body exceeds %d bytes", modelBodyByteLimit))
		return
	}

	// Tolerant of unknown fields — OpenAI request shapes evolve constantly
	// and we only need `model`.
	var head struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid JSON body: %w", err))
		return
	}
	if strings.TrimSpace(head.Model) == "" {
		writeErr(w, http.StatusBadRequest, errors.New("missing required field: model"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	m, err := g.ModelResolver.Resolve(ctx, head.Model)
	if errors.Is(err, ErrModelNotFound) {
		writeErr(w, http.StatusNotFound, fmt.Errorf("model %q not found", head.Model))
		recordModelInvocation(head.Model, http.StatusNotFound, start)
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		recordModelInvocation(head.Model, http.StatusBadGateway, start)
		return
	}

	target := modelServiceURL(m)
	if g.OverrideTarget != nil {
		target = g.OverrideTarget(target)
	}

	proxy := newReverseProxy(target)
	r2 := r.Clone(ctx)
	r2.Body = io.NopCloser(bytes.NewReader(body))
	r2.ContentLength = int64(len(body))
	r2.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	// Path stays as-is (/v1/chat/completions etc.) — llama-server speaks
	// OpenAI natively at the same paths.
	rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
	proxy.ServeHTTP(rec, r2)
	recordModelInvocation(head.Model, rec.code, start)
}

// modelServiceURL returns the in-cluster URL for a Model's Service.
func modelServiceURL(m *kryptonv1alpha1.Model) *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s.%s.svc:%d", m.Name, m.Namespace, m.Spec.Port),
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
