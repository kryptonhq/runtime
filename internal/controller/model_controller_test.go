/*
Copyright 2026 Krypton Authors.
*/

package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

func newModel() *kryptonv1alpha1.Model {
	return &kryptonv1alpha1.Model{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-small", Namespace: "models"},
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

func newModelReconciler(t *testing.T, objs ...client.Object) (*ModelReconciler, client.Client) {
	t.Helper()
	s := testScheme(t)
	cli := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&kryptonv1alpha1.Model{}).
		WithObjects(objs...).
		Build()
	return &ModelReconciler{Client: cli, Scheme: s}, cli
}

func reconcileModel(t *testing.T, r *ModelReconciler, name, ns string) {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

func TestModelReconcileCreatesChildren(t *testing.T) {
	m := newModel()
	r, cli := newModelReconciler(t, m)

	// First pass adds finalizer + requeues.
	reconcileModel(t, r, m.Name, m.Namespace)
	// Second pass creates children.
	reconcileModel(t, r, m.Name, m.Namespace)

	var deploy appsv1.Deployment
	if err := cli.Get(context.Background(), types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, &deploy); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if got := deploy.Spec.Template.Spec.Containers[0].Image; got != DefaultLlamaCppImage {
		t.Errorf("image = %q, want %q", got, DefaultLlamaCppImage)
	}
	args := deploy.Spec.Template.Spec.Containers[0].Args
	if !containsPair(args, "--hf-repo", m.Spec.Source.HuggingFace) {
		t.Errorf("missing --hf-repo flag: %v", args)
	}
	if !containsPair(args, "--hf-file", m.Spec.Source.File) {
		t.Errorf("missing --hf-file flag: %v", args)
	}
	if !containsPair(args, "--alias", m.Name) {
		t.Errorf("missing --alias flag: %v", args)
	}

	var svc corev1.Service
	if err := cli.Get(context.Background(), types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, &svc); err != nil {
		t.Fatalf("get service: %v", err)
	}
	if svc.Spec.Ports[0].Port != m.Spec.Port {
		t.Errorf("svc port = %d, want %d", svc.Spec.Ports[0].Port, m.Spec.Port)
	}

	var sa corev1.ServiceAccount
	if err := cli.Get(context.Background(), types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, &sa); err != nil {
		t.Fatalf("get sa: %v", err)
	}
}

func TestModelStatusURL(t *testing.T) {
	m := newModel()
	r, cli := newModelReconciler(t, m)
	reconcileModel(t, r, m.Name, m.Namespace)
	reconcileModel(t, r, m.Name, m.Namespace)

	var got kryptonv1alpha1.Model
	if err := cli.Get(context.Background(), types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, &got); err != nil {
		t.Fatalf("get model: %v", err)
	}
	want := "http://qwen-small.models.svc:8080/v1"
	if got.Status.URL != want {
		t.Errorf("status.url = %q, want %q", got.Status.URL, want)
	}
	if got.Status.Phase != kryptonv1alpha1.ModelPhasePending {
		t.Errorf("status.phase = %q, want Pending", got.Status.Phase)
	}
}

func containsPair(args []string, flag, val string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == val {
			return true
		}
	}
	return false
}
