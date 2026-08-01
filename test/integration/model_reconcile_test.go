//go:build envtest

/*
Copyright 2026 Krypton Authors.
*/

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

func newModel(name, ns string) *kryptonv1alpha1.Model {
	return &kryptonv1alpha1.Model{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: kryptonv1alpha1.ModelSpec{
			Source: kryptonv1alpha1.ModelSource{
				HuggingFace: "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
				File:        "qwen2.5-0.5b-instruct-q4_k_m.gguf",
			},
			Runtime:     kryptonv1alpha1.RuntimeLlamaCpp,
			Port:        8080,
			MinReplicas: 1,
			MaxReplicas: 1,
		},
	}
}

func TestModelReconcileCreatesChildren(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "qwen"}

	if err := k8sClient.Create(ctx, newModel(key.Name, ns)); err != nil {
		t.Fatalf("create model: %v", err)
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

	if len(deploy.OwnerReferences) != 1 {
		t.Fatalf("ownerReferences = %+v, want exactly one", deploy.OwnerReferences)
	}
	if deploy.OwnerReferences[0].Kind != "Model" {
		t.Errorf("owner kind = %q, want Model", deploy.OwnerReferences[0].Kind)
	}

	// Models serve a single container — no sidecar, because scale-to-zero
	// isn't supported for weights that take minutes to load.
	if len(deploy.Spec.Template.Spec.Containers) != 1 {
		t.Errorf("containers = %v, want exactly the runtime container",
			containerNames(deploy.Spec.Template.Spec.Containers))
	}
}

// The controller translates spec.source into llama.cpp CLI flags. Getting
// these wrong means the pod starts and then fails to find weights.
func TestModelLlamaCppArgsAreDerivedFromSpec(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "qwen-args"}

	model := newModel(key.Name, ns)
	model.Spec.Port = 9090
	model.Spec.Args = []string{"--ctx-size", "4096"}
	if err := k8sClient.Create(ctx, model); err != nil {
		t.Fatalf("create model: %v", err)
	}

	var deploy appsv1.Deployment
	eventually(t, "Deployment to be created", func() error {
		return k8sClient.Get(ctx, key, &deploy)
	})

	args := strings.Join(deploy.Spec.Template.Spec.Containers[0].Args, " ")
	for _, want := range []string{
		"--host 0.0.0.0",
		"--port 9090",
		"--hf-repo Qwen/Qwen2.5-0.5B-Instruct-GGUF",
		"--hf-file qwen2.5-0.5b-instruct-q4_k_m.gguf",
		"--alias qwen-args",
		// User args are appended after the generated ones so they can override.
		"--ctx-size 4096",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q\ngot: %s", want, args)
		}
	}
}

func TestModelStatusIsPopulated(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "qwen-status"}

	if err := k8sClient.Create(ctx, newModel(key.Name, ns)); err != nil {
		t.Fatalf("create model: %v", err)
	}

	eventually(t, "status to converge", func() error {
		var got kryptonv1alpha1.Model
		if err := k8sClient.Get(ctx, key, &got); err != nil {
			return err
		}
		if got.Status.ObservedGeneration != got.Generation {
			return fmt.Errorf("observedGeneration = %d, generation = %d",
				got.Status.ObservedGeneration, got.Generation)
		}
		if got.Status.URL == "" {
			return fmt.Errorf("status.url is empty")
		}
		return nil
	})

	var got kryptonv1alpha1.Model
	if err := k8sClient.Get(ctx, key, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	// The URL is the OpenAI base the gateway routes to; it must name the
	// in-cluster Service, not a pod IP.
	if !strings.Contains(got.Status.URL, key.Name) || !strings.Contains(got.Status.URL, ns) {
		t.Errorf("status.url = %q, want it to reference %s/%s", got.Status.URL, ns, key.Name)
	}
	// No kubelet, so Ready is unreachable here.
	if got.Status.Phase == kryptonv1alpha1.ModelPhaseReady {
		t.Error("phase = Ready, which envtest cannot legitimately reach (no kubelet)")
	}
}

// Custom images must win over the controller's built-in llama.cpp default,
// otherwise air-gapped installs can't point at a mirror.
func TestModelImageOverride(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "custom-image"}

	model := newModel(key.Name, ns)
	model.Spec.Image = "registry.internal/llama.cpp:pinned"
	if err := k8sClient.Create(ctx, model); err != nil {
		t.Fatalf("create model: %v", err)
	}

	eventually(t, "Deployment to use the overridden image", func() error {
		var d appsv1.Deployment
		if err := k8sClient.Get(ctx, key, &d); err != nil {
			return err
		}
		got := d.Spec.Template.Spec.Containers[0].Image
		if got != "registry.internal/llama.cpp:pinned" {
			return fmt.Errorf("image = %q", got)
		}
		return nil
	})
}
