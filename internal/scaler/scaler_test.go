/*
Copyright 2026 Krypton Authors.
*/

package scaler

import (
	"context"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

type fakeProbe struct {
	mu sync.Mutex
	n  map[types.NamespacedName]int
}

func (p *fakeProbe) AgentInflight(_ context.Context, key types.NamespacedName) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.n[key], nil
}

func (p *fakeProbe) set(key types.NamespacedName, n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.n == nil {
		p.n = map[types.NamespacedName]int{}
	}
	p.n[key] = n
}

func scalerScheme(t *testing.T) *runtime.Scheme {
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

func newAgent(name, ns string, mut func(*kryptonv1alpha1.Agent)) *kryptonv1alpha1.Agent {
	a := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: kryptonv1alpha1.AgentSpec{
			Mode:             kryptonv1alpha1.ModeServerless,
			Concurrency:      8,
			MinReplicas:      0,
			MaxReplicas:      10,
			ScaleToZeroAfter: metav1.Duration{Duration: 5 * time.Minute},
		},
	}
	if mut != nil {
		mut(a)
	}
	return a
}

func newScaler(t *testing.T, objs ...client.Object) (*Scaler, client.Client, *fakeProbe) {
	t.Helper()
	cli := fake.NewClientBuilder().
		WithScheme(scalerScheme(t)).
		WithStatusSubresource(&kryptonv1alpha1.Agent{}).
		WithObjects(objs...).
		Build()
	p := &fakeProbe{}
	s := &Scaler{Client: cli, Probe: p}
	return s, cli, p
}

func TestReconcileOnePatchesStatus(t *testing.T) {
	a := newAgent("travel", "agents", func(a *kryptonv1alpha1.Agent) {
		a.Status.LastInvocationAt = &metav1.Time{Time: time.Now()}
	})
	s, cli, p := newScaler(t, a)
	p.set(types.NamespacedName{Name: "travel", Namespace: "agents"}, 20)

	if err := s.reconcileOne(context.Background(), a); err != nil {
		t.Fatalf("reconcileOne: %v", err)
	}

	got := &kryptonv1alpha1.Agent{}
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "travel", Namespace: "agents"}, got); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	// 20 / 8 = 3 (ceil)
	if got.Status.DesiredReplicas != 3 {
		t.Errorf("desiredReplicas = %d, want 3", got.Status.DesiredReplicas)
	}
}

func TestReconcileOneNoOpWhenAlreadyAtDesired(t *testing.T) {
	a := newAgent("travel", "agents", func(a *kryptonv1alpha1.Agent) {
		a.Status.DesiredReplicas = 0
	})
	s, cli, _ := newScaler(t, a)

	// First call patches nothing (already 0).
	if err := s.reconcileOne(context.Background(), a); err != nil {
		t.Fatalf("reconcileOne: %v", err)
	}
	// Verify by reading and checking resource version didn't bump in any
	// observable way — proxy by verifying desired stays 0.
	got := &kryptonv1alpha1.Agent{}
	_ = cli.Get(context.Background(), types.NamespacedName{Name: "travel", Namespace: "agents"}, got)
	if got.Status.DesiredReplicas != 0 {
		t.Errorf("unexpected desiredReplicas: %d", got.Status.DesiredReplicas)
	}
}

func TestScaleUpRecordedForHysteresis(t *testing.T) {
	a := newAgent("travel", "agents", func(a *kryptonv1alpha1.Agent) {
		a.Status.LastInvocationAt = &metav1.Time{Time: time.Now()}
		a.Status.DesiredReplicas = 0
	})
	s, _, p := newScaler(t, a)
	key := types.NamespacedName{Name: "travel", Namespace: "agents"}
	p.set(key, 32) // → 4

	if err := s.reconcileOne(context.Background(), a); err != nil {
		t.Fatalf("reconcileOne: %v", err)
	}
	if got := s.getLastScaleUp(key); got.IsZero() {
		t.Error("expected lastScaleUp recorded on scale up")
	}
}

func TestTickPatchesAllAgents(t *testing.T) {
	a := newAgent("travel", "agents", func(a *kryptonv1alpha1.Agent) {
		a.Status.LastInvocationAt = &metav1.Time{Time: time.Now()}
	})
	b := newAgent("billing", "agents", func(a *kryptonv1alpha1.Agent) {
		a.Status.LastInvocationAt = &metav1.Time{Time: time.Now()}
	})
	s, cli, p := newScaler(t, a, b)
	p.set(types.NamespacedName{Name: "travel", Namespace: "agents"}, 16)  // → 2
	p.set(types.NamespacedName{Name: "billing", Namespace: "agents"}, 24) // → 3

	s.tick(context.Background())

	got := &kryptonv1alpha1.Agent{}
	_ = cli.Get(context.Background(), types.NamespacedName{Name: "travel", Namespace: "agents"}, got)
	if got.Status.DesiredReplicas != 2 {
		t.Errorf("travel: %d, want 2", got.Status.DesiredReplicas)
	}
	_ = cli.Get(context.Background(), types.NamespacedName{Name: "billing", Namespace: "agents"}, got)
	if got.Status.DesiredReplicas != 3 {
		t.Errorf("billing: %d, want 3", got.Status.DesiredReplicas)
	}
}
