/*
Copyright 2026 Krypton Authors.
*/

package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("clientgo scheme: %v", err)
	}
	if err := kryptonv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("krypton scheme: %v", err)
	}
	return s
}

func newAgent() *kryptonv1alpha1.Agent {
	a := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "travel-agent",
			Namespace: "agents",
		},
		Spec: kryptonv1alpha1.AgentSpec{
			Image:          "ghcr.io/org/travel-agent:latest",
			Mode:           kryptonv1alpha1.ModeServerless,
			Protocol:       kryptonv1alpha1.ProtocolA2A,
			Port:           8080,
			InvocationPath: "/a2a",
			Concurrency:    8,
			MaxReplicas:    10,
			MinReplicas:    0,
		},
	}
	return a
}

func newReconciler(t *testing.T, objs ...client.Object) (*AgentReconciler, client.Client) {
	t.Helper()
	s := testScheme(t)
	cli := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&kryptonv1alpha1.Agent{}).
		WithObjects(objs...).
		Build()
	return &AgentReconciler{Client: cli, Scheme: s, ProxyImage: "ghcr.io/kryptonhq/krypton-proxy:test"}, cli
}

func reconcile(t *testing.T, r *AgentReconciler, name, ns string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	return res
}

func TestReconcileCreatesChildren(t *testing.T) {
	agent := newAgent()
	r, cli := newReconciler(t, agent)
	ctx := context.Background()

	// First pass adds finalizer and requeues.
	if res := reconcile(t, r, agent.Name, agent.Namespace); !res.Requeue {
		t.Fatalf("first reconcile should requeue to apply finalizer, got %+v", res)
	}

	// Second pass creates child resources.
	reconcile(t, r, agent.Name, agent.Namespace)

	deploy := &appsv1.Deployment{}
	if err := cli.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if got := deploy.Spec.Template.Spec.Containers[0].Image; got != agent.Spec.Image {
		t.Errorf("image = %q, want %q", got, agent.Spec.Image)
	}
	if got := deploy.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort; got != agent.Spec.Port {
		t.Errorf("port = %d, want %d", got, agent.Spec.Port)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 0 {
		t.Errorf("replicas = %v, want 0 (minReplicas)", deploy.Spec.Replicas)
	}

	svc := &corev1.Service{}
	if err := cli.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, svc); err != nil {
		t.Fatalf("get service: %v", err)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != agent.Spec.Port {
		t.Errorf("unexpected svc ports: %+v", svc.Spec.Ports)
	}
	if got := svc.Spec.Ports[0].TargetPort; got != intstr.FromString(sidecarPortName) {
		t.Errorf("svc TargetPort = %v, want named %q", got, sidecarPortName)
	}

	sa := &corev1.ServiceAccount{}
	if err := cli.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, sa); err != nil {
		t.Fatalf("get sa: %v", err)
	}
}

func TestReconcileStatusReady(t *testing.T) {
	agent := newAgent()
	agent.Status.DesiredReplicas = 1
	r, cli := newReconciler(t, agent)
	ctx := context.Background()

	reconcile(t, r, agent.Name, agent.Namespace) // adds finalizer
	reconcile(t, r, agent.Name, agent.Namespace) // creates children

	// Simulate apps controller marking the deployment ready.
	deploy := &appsv1.Deployment{}
	if err := cli.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	deploy.Status.Replicas = 1
	deploy.Status.ReadyReplicas = 1
	if err := cli.Status().Update(ctx, deploy); err != nil {
		t.Fatalf("update deploy status: %v", err)
	}

	reconcile(t, r, agent.Name, agent.Namespace)

	got := &kryptonv1alpha1.Agent{}
	if err := cli.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, got); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status.Phase != kryptonv1alpha1.PhaseReady {
		t.Errorf("phase = %q, want Ready", got.Status.Phase)
	}
	if got.Status.ReadyReplicas != 1 {
		t.Errorf("readyReplicas = %d, want 1", got.Status.ReadyReplicas)
	}
	if got.Status.URL == "" {
		t.Errorf("status.URL not populated")
	}
}

func TestReconcileScaledToZeroIsAvailable(t *testing.T) {
	agent := newAgent()
	r, cli := newReconciler(t, agent)
	ctx := context.Background()

	reconcile(t, r, agent.Name, agent.Namespace)
	reconcile(t, r, agent.Name, agent.Namespace)

	got := &kryptonv1alpha1.Agent{}
	if err := cli.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, got); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status.Phase != kryptonv1alpha1.PhaseReady {
		t.Errorf("phase = %q, want Ready (scaled-to-zero is Available)", got.Status.Phase)
	}
	if !hasCondition(got.Status.Conditions, kryptonv1alpha1.ConditionAvailable, metav1.ConditionTrue) {
		t.Errorf("expected Available=True condition, got %+v", got.Status.Conditions)
	}
}

func TestReconcileDeleteRemovesFinalizer(t *testing.T) {
	agent := newAgent()
	agent.Finalizers = []string{FinalizerName}
	now := metav1.Now()
	agent.DeletionTimestamp = &now
	r, cli := newReconciler(t, agent)
	ctx := context.Background()

	reconcile(t, r, agent.Name, agent.Namespace)

	got := &kryptonv1alpha1.Agent{}
	err := cli.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, got)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return // fake client may have GC'd after finalizer removal
		}
		t.Fatalf("get agent: %v", err)
	}
	for _, f := range got.Finalizers {
		if f == FinalizerName {
			t.Fatalf("finalizer still present: %v", got.Finalizers)
		}
	}
}

func TestReconcileImageDriftRolls(t *testing.T) {
	agent := newAgent()
	r, cli := newReconciler(t, agent)
	ctx := context.Background()

	reconcile(t, r, agent.Name, agent.Namespace)
	reconcile(t, r, agent.Name, agent.Namespace)

	// Bump the image.
	got := &kryptonv1alpha1.Agent{}
	if err := cli.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, got); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	got.Spec.Image = "ghcr.io/org/travel-agent:v2"
	if err := cli.Update(ctx, got); err != nil {
		t.Fatalf("update agent: %v", err)
	}

	reconcile(t, r, got.Name, got.Namespace)

	deploy := &appsv1.Deployment{}
	if err := cli.Get(ctx, types.NamespacedName{Name: got.Name, Namespace: got.Namespace}, deploy); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if deploy.Spec.Template.Spec.Containers[0].Image != "ghcr.io/org/travel-agent:v2" {
		t.Errorf("deployment image not updated: %q", deploy.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestReconcileInjectsSidecar(t *testing.T) {
	agent := newAgent()
	r, cli := newReconciler(t, agent)
	ctx := context.Background()

	reconcile(t, r, agent.Name, agent.Namespace) // adds finalizer
	reconcile(t, r, agent.Name, agent.Namespace) // creates children

	deploy := &appsv1.Deployment{}
	if err := cli.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	containers := deploy.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("containers = %d, want 2 (agent + proxy)", len(containers))
	}
	var proxy *corev1.Container
	for i := range containers {
		if containers[i].Name == sidecarContainerName {
			proxy = &containers[i]
			break
		}
	}
	if proxy == nil {
		t.Fatal("krypton-proxy sidecar not injected")
	}
	if proxy.Image != "ghcr.io/kryptonhq/krypton-proxy:test" {
		t.Errorf("proxy image = %q, want test override", proxy.Image)
	}
	if len(proxy.Ports) != 1 || proxy.Ports[0].ContainerPort != sidecarPort {
		t.Errorf("proxy port = %+v, want %d", proxy.Ports, sidecarPort)
	}
	wantEnv := map[string]string{
		"KRYPTON_AGENT_NAME":      agent.Name,
		"KRYPTON_AGENT_NAMESPACE": agent.Namespace,
		"KRYPTON_CONCURRENCY":     "8",
		"KRYPTON_MODE":            "serverless",
	}
	got := map[string]string{}
	for _, e := range proxy.Env {
		got[e.Name] = e.Value
	}
	for k, v := range wantEnv {
		if got[k] != v {
			t.Errorf("proxy env %s = %q, want %q", k, got[k], v)
		}
	}
	if proxy.ReadinessProbe == nil || proxy.LivenessProbe == nil {
		t.Error("proxy missing readiness/liveness probes")
	}
}

func hasCondition(conds []metav1.Condition, t string, want metav1.ConditionStatus) bool {
	for _, c := range conds {
		if c.Type == t {
			return c.Status == want
		}
	}
	return false
}
