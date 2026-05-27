/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package gateway implements the Krypton public ingress.
//
// It has two responsibilities:
//   - Routing: incoming invocations go to the right Agent pod
//   - Activation: when an Agent has zero ready replicas, buffer the request,
//     patch status.desiredReplicas, and wait for the pod to come up
//
// The gateway is in front of the cluster — operators terminate TLS at their
// own ingress (Envoy, Nginx, ALB, Cloudflare) and route traffic into the
// gateway over plain HTTP.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/metrics"
)

// ErrBufferFull is returned when an agent's cold-start buffer is full. The
// gateway translates it to 503 + Retry-After.
var ErrBufferFull = errors.New("activator buffer full")

// ErrColdStartTimeout is returned when an agent failed to reach ready within
// the configured startup timeout.
var ErrColdStartTimeout = errors.New("cold start timed out")

// Activator owns cold-start handling for serverless agents.
type Activator struct {
	Client client.Client

	// MaxBufferPerAgent caps concurrent cold-start waiters per agent. Over
	// the cap returns ErrBufferFull.
	MaxBufferPerAgent int

	// PollInterval is how often the activator re-checks for ready endpoints
	// during a cold start. 50ms keeps cold-start latency low without
	// hammering the cache.
	PollInterval time.Duration

	// Default fallback timeout when spec.StartupTimeout is 0.
	DefaultStartupTimeout time.Duration

	mu     sync.Mutex
	depths map[types.NamespacedName]int
}

// Resolve returns an upstream URL for the given agent, blocking through a
// cold start if necessary. It returns ErrBufferFull if too many requests are
// already waiting on this agent, and ErrColdStartTimeout if the pod never
// became ready. The bool reports whether the request hit the cold path.
func (a *Activator) Resolve(ctx context.Context, key types.NamespacedName) (*url.URL, bool, error) {
	logger := log.FromContext(ctx).WithValues("agent", key)

	agent, err := a.fetchAgent(ctx, key)
	if err != nil {
		return nil, false, err
	}
	target := serviceURL(agent)

	if a.hasReadyEndpoints(ctx, key) {
		return target, false, nil
	}

	// Cold path — buffer and scale up.
	if !a.acquireSlot(key) {
		return nil, true, ErrBufferFull
	}
	defer a.releaseSlot(key)
	metrics.ColdStartsTotal.WithLabelValues(key.Name, key.Namespace).Inc()

	if err := a.scaleUp(ctx, agent); err != nil {
		return nil, true, fmt.Errorf("scale up: %w", err)
	}

	timeout := agent.Spec.StartupTimeout.Duration
	if timeout == 0 {
		timeout = a.DefaultStartupTimeout
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := a.waitForEndpoints(waitCtx, key); err != nil {
		return nil, true, err
	}
	logger.V(1).Info("cold start complete")
	return target, true, nil
}

// RecordInvocation patches status.LastInvocationAt to "now". Called by the
// gateway after a successful invocation so the scaler can tell apart
// "never used" from "recently active" agents. Best-effort — errors are
// logged but not surfaced to the caller.
func (a *Activator) RecordInvocation(ctx context.Context, key types.NamespacedName) {
	var agent kryptonv1alpha1.Agent
	if err := a.Client.Get(ctx, key, &agent); err != nil {
		return
	}
	patch := client.MergeFrom(agent.DeepCopy())
	now := metav1.Now()
	agent.Status.LastInvocationAt = &now
	if err := a.Client.Status().Patch(ctx, &agent, patch); err != nil {
		log.FromContext(ctx).V(1).Info("record invocation failed", "agent", key, "error", err.Error())
	}
}

func (a *Activator) fetchAgent(ctx context.Context, key types.NamespacedName) (*kryptonv1alpha1.Agent, error) {
	var agent kryptonv1alpha1.Agent
	if err := a.Client.Get(ctx, key, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (a *Activator) hasReadyEndpoints(ctx context.Context, key types.NamespacedName) bool {
	var eps corev1.Endpoints
	if err := a.Client.Get(ctx, key, &eps); err != nil {
		return false
	}
	for _, s := range eps.Subsets {
		if len(s.Addresses) > 0 {
			return true
		}
	}
	return false
}

func (a *Activator) acquireSlot(key types.NamespacedName) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.depths == nil {
		a.depths = map[types.NamespacedName]int{}
	}
	if a.depths[key] >= a.MaxBufferPerAgent {
		return false
	}
	a.depths[key]++
	metrics.BufferDepth.WithLabelValues(key.Name, key.Namespace).Set(float64(a.depths[key]))
	return true
}

func (a *Activator) releaseSlot(key types.NamespacedName) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.depths[key] > 0 {
		a.depths[key]--
	}
	metrics.BufferDepth.WithLabelValues(key.Name, key.Namespace).Set(float64(a.depths[key]))
}

// BufferDepth is exposed for metrics + tests.
func (a *Activator) BufferDepth(key types.NamespacedName) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.depths[key]
}

// scaleUp patches status.desiredReplicas = 1 and bumps LastInvocationAt
// so the scaler treats the agent as recently active and doesn't race us
// back to zero before the cold-started pod is ready. Best-effort: a
// conflict gets retried once because two activators racing the same
// agent is the expected case under load.
func (a *Activator) scaleUp(ctx context.Context, agent *kryptonv1alpha1.Agent) error {
	if agent.Status.DesiredReplicas >= 1 && agent.Status.LastInvocationAt != nil {
		return nil
	}
	patch := client.MergeFrom(agent.DeepCopy())
	if agent.Status.DesiredReplicas < 1 {
		agent.Status.DesiredReplicas = 1
	}
	now := metav1.Now()
	agent.Status.LastInvocationAt = &now
	err := a.Client.Status().Patch(ctx, agent, patch)
	if apierrors.IsConflict(err) {
		// Re-fetch and retry once.
		fresh, fetchErr := a.fetchAgent(ctx, client.ObjectKeyFromObject(agent))
		if fetchErr != nil {
			return fetchErr
		}
		if fresh.Status.DesiredReplicas >= 1 && fresh.Status.LastInvocationAt != nil {
			return nil
		}
		patch = client.MergeFrom(fresh.DeepCopy())
		if fresh.Status.DesiredReplicas < 1 {
			fresh.Status.DesiredReplicas = 1
		}
		fresh.Status.LastInvocationAt = &now
		return a.Client.Status().Patch(ctx, fresh, patch)
	}
	return err
}

func (a *Activator) waitForEndpoints(ctx context.Context, key types.NamespacedName) error {
	tick := time.NewTicker(a.PollInterval)
	defer tick.Stop()
	if a.hasReadyEndpoints(ctx, key) {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrColdStartTimeout
			}
			return ctx.Err()
		case <-tick.C:
			if a.hasReadyEndpoints(ctx, key) {
				return nil
			}
		}
	}
}

// serviceURL returns the in-cluster URL for an Agent's Service.
func serviceURL(agent *kryptonv1alpha1.Agent) *url.URL {
	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s.%s.svc:%d", agent.Name, agent.Namespace, agent.Spec.Port),
	}
	return u
}
