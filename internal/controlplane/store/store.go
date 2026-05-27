/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package store persists an Agent registry mirror. CRDs remain the source
// of truth — the store is a side-effect of reconciliation, suitable for
// offline tooling, dashboards, and (later) invocation/metrics history.
//
// Two implementations:
//   - Memory: lock-free-enough map, useful for tests and dev mode
//   - Postgres: production target, uses pgx and an embedded schema
package store

import (
	"context"

	"k8s.io/apimachinery/pkg/types"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

// Store is the contract every persistence backend implements.
type Store interface {
	// Upsert mirrors the agent CR's current spec and status.
	Upsert(ctx context.Context, a *kryptonv1alpha1.Agent) error
	// Delete removes the agent from the store (idempotent — no-op if absent).
	Delete(ctx context.Context, key types.NamespacedName) error
	// Get returns ErrNotFound if the agent isn't stored.
	Get(ctx context.Context, key types.NamespacedName) (*Record, error)
	// List enumerates agents. namespace == "" returns all namespaces.
	List(ctx context.Context, namespace string) ([]Record, error)
	// Close releases backing resources (DB connections, etc.).
	Close() error
}

// Record is what the store hands back — the parts that matter for offline
// querying without the full K8s object envelope.
type Record struct {
	UID       string
	Namespace string
	Name      string
	Spec      kryptonv1alpha1.AgentSpec
	Status    kryptonv1alpha1.AgentStatus
}

// ErrNotFound is returned by Get when the agent isn't stored.
var ErrNotFound = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "agent not found in store" }
