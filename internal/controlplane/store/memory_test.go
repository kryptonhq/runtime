/*
Copyright 2026 Krypton Authors.
*/

package store

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

func newAgent(name, ns, uid string) *kryptonv1alpha1.Agent {
	return &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID(uid)},
		Spec: kryptonv1alpha1.AgentSpec{
			Image: "ghcr.io/org/" + name + ":latest",
			Port:  8080,
		},
		Status: kryptonv1alpha1.AgentStatus{Phase: kryptonv1alpha1.PhaseReady},
	}
}

// TestStoreContract exercises the shared Store contract. We run it against
// Memory here; the Postgres impl runs the same test with the integration
// build tag (postgres_test.go).
func TestStoreContract(t *testing.T) { testStore(t, NewMemory()) }

func testStore(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() { _ = s.Close() })

	a := newAgent("travel", "agents", "uid-travel")
	b := newAgent("billing", "agents", "uid-billing")
	c := newAgent("search", "other", "uid-search")

	for _, ag := range []*kryptonv1alpha1.Agent{a, b, c} {
		if err := s.Upsert(ctx, ag); err != nil {
			t.Fatalf("upsert %s: %v", ag.Name, err)
		}
	}

	// Get
	got, err := s.Get(ctx, types.NamespacedName{Namespace: "agents", Name: "travel"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UID != "uid-travel" || got.Spec.Image != "ghcr.io/org/travel:latest" {
		t.Errorf("unexpected record: %+v", got)
	}

	// Get missing
	_, err = s.Get(ctx, types.NamespacedName{Namespace: "agents", Name: "missing"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("missing key returned %v, want ErrNotFound", err)
	}

	// List all
	all, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("list all = %d, want 3", len(all))
	}

	// List filtered
	filtered, err := s.List(ctx, "agents")
	if err != nil {
		t.Fatalf("list ns: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("list ns = %d, want 2", len(filtered))
	}

	// Upsert applies updates
	a.Status.Replicas = 5
	if err := s.Upsert(ctx, a); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, _ = s.Get(ctx, types.NamespacedName{Namespace: "agents", Name: "travel"})
	if got.Status.Replicas != 5 {
		t.Errorf("status not updated: %d", got.Status.Replicas)
	}

	// A deleted-and-recreated Agent keeps its namespace/name but gets a new
	// UID from Kubernetes. Upsert must treat (namespace, name) as identity
	// and adopt the new UID, not fail or create a second row.
	//
	// This bites when the control plane isn't running to observe the delete
	// (restart spanning a delete+apply, GitOps prune-and-recreate), so the
	// stale row is still present when the new object is first seen.
	recreated := newAgent("travel", "agents", "uid-travel-v2")
	recreated.Status.Replicas = 7
	if err := s.Upsert(ctx, recreated); err != nil {
		t.Fatalf("upsert after delete+recreate (new UID, same namespace/name): %v", err)
	}
	got, err = s.Get(ctx, types.NamespacedName{Namespace: "agents", Name: "travel"})
	if err != nil {
		t.Fatalf("get after recreate: %v", err)
	}
	if got.UID != "uid-travel-v2" {
		t.Errorf("UID = %q, want the recreated agent's uid-travel-v2", got.UID)
	}
	if got.Status.Replicas != 7 {
		t.Errorf("Replicas = %d, want 7", got.Status.Replicas)
	}
	// Still exactly one row for that namespace/name.
	afterRecreate, err := s.List(ctx, "agents")
	if err != nil {
		t.Fatalf("list after recreate: %v", err)
	}
	travelRows := 0
	for _, r := range afterRecreate {
		if r.Name == "travel" {
			travelRows++
		}
	}
	if travelRows != 1 {
		t.Errorf("found %d rows for agents/travel, want exactly 1", travelRows)
	}

	// Delete is idempotent
	key := types.NamespacedName{Namespace: "agents", Name: "billing"}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Errorf("second delete should be no-op, got %v", err)
	}
	if _, err := s.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete, get returned %v, want ErrNotFound", err)
	}
}
