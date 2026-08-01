//go:build envtest

/*
Copyright 2026 Krypton Authors.
*/

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/controller"
)

func newAgent(name, ns string) *kryptonv1alpha1.Agent {
	return &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: kryptonv1alpha1.AgentSpec{
			Image:          "ghcr.io/org/travel-agent:v1",
			Mode:           kryptonv1alpha1.ModeAlwaysOn,
			Protocol:       kryptonv1alpha1.ProtocolA2A,
			Port:           8080,
			InvocationPath: "/a2a",
			Concurrency:    8,
			MinReplicas:    1,
			MaxReplicas:    5,
		},
	}
}

// The full create path, driven by a real manager reacting to a real watch
// event rather than a hand-called Reconcile.
func TestAgentReconcileCreatesChildren(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "travel"}

	if err := k8sClient.Create(ctx, newAgent(key.Name, ns)); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	var deploy appsv1.Deployment
	eventually(t, "Deployment to be created", func() error {
		return k8sClient.Get(ctx, key, &deploy)
	})

	var svc corev1.Service
	eventually(t, "Service to be created", func() error {
		return k8sClient.Get(ctx, key, &svc)
	})

	var sa corev1.ServiceAccount
	eventually(t, "ServiceAccount to be created", func() error {
		return k8sClient.Get(ctx, key, &sa)
	})

	// The sidecar must be injected alongside the user container — this is
	// what makes concurrency enforcement and load reporting work.
	names := containerNames(deploy.Spec.Template.Spec.Containers)
	if len(deploy.Spec.Template.Spec.Containers) != 2 {
		t.Errorf("containers = %v, want the user container plus krypton-proxy", names)
	}
	if !contains(names, "krypton-proxy") {
		t.Errorf("krypton-proxy sidecar not injected; containers = %v", names)
	}

	// Ownership: assert the reference is SET. envtest has no garbage
	// collector, so never assert the child is deleted with the parent.
	if len(deploy.OwnerReferences) != 1 {
		t.Fatalf("Deployment ownerReferences = %+v, want exactly one", deploy.OwnerReferences)
	}
	owner := deploy.OwnerReferences[0]
	if owner.Kind != "Agent" || owner.Name != key.Name {
		t.Errorf("owner = %s/%s, want Agent/%s", owner.Kind, owner.Name, key.Name)
	}
	if owner.Controller == nil || !*owner.Controller {
		t.Error("ownerReference.controller should be true so the Owns() watch fires")
	}
}

// The finalizer is what lets reconcileDelete drain children before the
// object disappears. Without it, deletion races child cleanup.
func TestAgentGetsFinalizer(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "finalized"}

	if err := k8sClient.Create(ctx, newAgent(key.Name, ns)); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	eventually(t, "finalizer to be added", func() error {
		var agent kryptonv1alpha1.Agent
		if err := k8sClient.Get(ctx, key, &agent); err != nil {
			return err
		}
		for _, f := range agent.Finalizers {
			if f == controller.FinalizerName {
				return nil
			}
		}
		return fmt.Errorf("finalizers = %v, want %q", agent.Finalizers, controller.FinalizerName)
	})
}

// Deletion must actually complete: the finalizer has to be removed by
// reconcileDelete, or the object hangs in Terminating forever. This is the
// single most common operator bug and it is invisible to fake-client tests,
// which don't honour finalizers.
func TestAgentDeletionCompletes(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "deleteme"}

	if err := k8sClient.Create(ctx, newAgent(key.Name, ns)); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Wait until it's fully reconciled before deleting.
	eventually(t, "Deployment to exist before delete", func() error {
		var d appsv1.Deployment
		return k8sClient.Get(ctx, key, &d)
	})

	var agent kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, key, &agent); err != nil {
		t.Fatalf("get before delete: %v", err)
	}
	if err := k8sClient.Delete(ctx, &agent); err != nil {
		t.Fatalf("delete: %v", err)
	}

	eventually(t, "Agent to be fully removed (finalizer released)", func() error {
		var a kryptonv1alpha1.Agent
		err := k8sClient.Get(ctx, key, &a)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("still present with finalizers %v and deletionTimestamp %v",
			a.Finalizers, a.DeletionTimestamp)
	})
}

// Status is written through the status subresource and reflects the
// observed generation. In envtest the Deployment never reports ready
// replicas (no kubelet), so Phase stays Pending — asserting Ready here
// would be asserting something envtest cannot produce.
func TestAgentStatusIsPopulated(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "statusy"}

	agent := newAgent(key.Name, ns)
	if err := k8sClient.Create(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	eventually(t, "status.observedGeneration to catch up", func() error {
		var got kryptonv1alpha1.Agent
		if err := k8sClient.Get(ctx, key, &got); err != nil {
			return err
		}
		if got.Status.ObservedGeneration != got.Generation {
			return fmt.Errorf("observedGeneration = %d, generation = %d",
				got.Status.ObservedGeneration, got.Generation)
		}
		if got.Status.Phase == "" {
			return fmt.Errorf("phase is still empty")
		}
		return nil
	})

	var got kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, key, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	// No kubelet -> no ready pods -> not Ready. Pending or Scaling are the
	// only legitimate outcomes in this environment.
	if got.Status.Phase == kryptonv1alpha1.PhaseReady {
		t.Errorf("phase = Ready, which envtest cannot legitimately reach (no kubelet)")
	}
	if got.Status.URL == "" {
		t.Error("status.url should be set from the Service DNS name regardless of readiness")
	}
}

// A spec edit must be picked up and applied to the Deployment. This is the
// watch-plus-requeue path that SetupWithManager wires.
func TestAgentSpecUpdatePropagatesToDeployment(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "mutable"}

	if err := k8sClient.Create(ctx, newAgent(key.Name, ns)); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	eventually(t, "initial Deployment", func() error {
		var d appsv1.Deployment
		return k8sClient.Get(ctx, key, &d)
	})

	// Retry the update to absorb the resourceVersion conflict that races
	// the controller's own finalizer/status writes. Real clients must do
	// this; the fake client never surfaces the conflict.
	eventually(t, "image update to be accepted", func() error {
		var agent kryptonv1alpha1.Agent
		if err := k8sClient.Get(ctx, key, &agent); err != nil {
			return err
		}
		agent.Spec.Image = "ghcr.io/org/travel-agent:v2"
		return k8sClient.Update(ctx, &agent)
	})

	eventually(t, "Deployment to pick up the new image", func() error {
		var d appsv1.Deployment
		if err := k8sClient.Get(ctx, key, &d); err != nil {
			return err
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Image == "ghcr.io/org/travel-agent:v2" {
				return nil
			}
		}
		return fmt.Errorf("images = %v, want one at :v2", containerImages(d.Spec.Template.Spec.Containers))
	})
}

// Once converged, the controller must stop writing. A status hot-loop is
// invisible in unit tests but hammers the API server in production.
func TestAgentStatusStopsChurning(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "settled"}

	if err := k8sClient.Create(ctx, newAgent(key.Name, ns)); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	eventually(t, "reconcile to settle", func() error {
		var a kryptonv1alpha1.Agent
		if err := k8sClient.Get(ctx, key, &a); err != nil {
			return err
		}
		if a.Status.ObservedGeneration != a.Generation {
			return fmt.Errorf("not yet converged")
		}
		return nil
	})

	// Let it quiesce, then snapshot the resourceVersion.
	time.Sleep(1 * time.Second)
	var before kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, key, &before); err != nil {
		t.Fatalf("get: %v", err)
	}

	consistently(t, "resourceVersion", 2*time.Second, func() error {
		var now kryptonv1alpha1.Agent
		if err := k8sClient.Get(ctx, key, &now); err != nil {
			return err
		}
		if now.ResourceVersion != before.ResourceVersion {
			return fmt.Errorf("resourceVersion moved %s -> %s; the controller is rewriting a converged object",
				before.ResourceVersion, now.ResourceVersion)
		}
		return nil
	})
}

// ---- helpers ---------------------------------------------------------------

func containerNames(cs []corev1.Container) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func containerImages(cs []corev1.Container) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Image)
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
