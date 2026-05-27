/*
Copyright 2026 Krypton Authors.
*/

package controlplane

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/controlplane/store"
)

func syncerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if err := kryptonv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("krypton scheme: %v", err)
	}
	return s
}

func TestSyncerUpsertsOnReconcile(t *testing.T) {
	a := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "travel", Namespace: "agents", UID: "u1"},
		Spec:       kryptonv1alpha1.AgentSpec{Image: "img:1", Port: 8080},
		Status:     kryptonv1alpha1.AgentStatus{Phase: kryptonv1alpha1.PhaseReady, Replicas: 2},
	}
	cli := fake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(a).Build()
	mem := store.NewMemory()
	syn := &Syncer{Client: cli, Store: mem}

	if _, err := syn.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "travel", Namespace: "agents"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := mem.Get(context.Background(), types.NamespacedName{Name: "travel", Namespace: "agents"})
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if got.Spec.Image != "img:1" || got.Status.Replicas != 2 {
		t.Errorf("unexpected stored record: %+v", got)
	}
}

func TestSyncerDeletesWhenAgentGone(t *testing.T) {
	// No agent in the fake client → NotFound on Get → Delete from store.
	cli := fake.NewClientBuilder().WithScheme(syncerScheme(t)).Build()
	mem := store.NewMemory()
	// Pre-seed the store so we can observe the delete.
	seed := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "travel", Namespace: "agents", UID: "u1"},
		Spec:       kryptonv1alpha1.AgentSpec{Image: "img:1"},
	}
	if err := mem.Upsert(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	syn := &Syncer{Client: cli, Store: mem}
	if _, err := syn.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "travel", Namespace: "agents"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, err := mem.Get(context.Background(), types.NamespacedName{Name: "travel", Namespace: "agents"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("agent still present after delete; err = %v", err)
	}
}

// Compile-time guarantee that Memory satisfies the Store contract.
var _ store.Store = (*store.Memory)(nil)
