/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controlplane

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/controlplane/store"
)

// Syncer is a controller-runtime Reconciler that mirrors Agent CRs into a
// Store. It's a write-through audit log: the API still serves from the
// informer cache (fresher), and the store is for offline querying and
// (later) joining against invocation/metrics history.
type Syncer struct {
	client.Client
	Store store.Store
}

// Reconcile applies the current cluster state of one Agent to the store.
// Deletions arrive as NotFound; missing rows in the store are tolerated by
// the idempotent Delete contract.
func (s *Syncer) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("agent", req.NamespacedName)

	var agent kryptonv1alpha1.Agent
	err := s.Get(ctx, req.NamespacedName, &agent)
	if apierrors.IsNotFound(err) {
		if delErr := s.Store.Delete(ctx, req.NamespacedName); delErr != nil {
			return ctrl.Result{}, fmt.Errorf("delete from store: %w", delErr)
		}
		logger.V(1).Info("removed from store")
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get agent: %w", err)
	}
	if err := s.Store.Upsert(ctx, &agent); err != nil {
		return ctrl.Result{}, fmt.Errorf("upsert into store: %w", err)
	}
	logger.V(1).Info("synced to store")
	return ctrl.Result{}, nil
}

// SetupWithManager wires the syncer into a controller-runtime manager.
func (s *Syncer) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("agent-store-syncer").
		For(&kryptonv1alpha1.Agent{}).
		Complete(s)
}
