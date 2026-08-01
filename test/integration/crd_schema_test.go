//go:build envtest

/*
Copyright 2026 Krypton Authors.
*/

package integration

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	resourceapi "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

// This is the test that makes hand-written CRDs safe.
//
// Every field of AgentSpec is populated, written through the real API
// server, and read back. Any field missing from the CRD's openAPIV3Schema
// gets pruned on write, so it comes back zero and this test fails naming
// the field. The fake-client unit tests cannot catch this: they never
// serialize through a schema.
func TestAgentSpecSurvivesRoundTrip(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)

	want := kryptonv1alpha1.AgentSpec{
		Image:            "ghcr.io/org/travel-agent:v1.2.3",
		ImagePullPolicy:  corev1.PullAlways,
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "ghcr-creds"}},
		Runtime:          "python",
		Framework:        "langgraph",
		Protocol:         kryptonv1alpha1.ProtocolMCP,
		Mode:             kryptonv1alpha1.ModeServerless,
		ScaleToZeroAfter: metav1.Duration{Duration: 120 * time.Second},
		// Deliberately non-zero: a zero int32 is elided by omitempty and
		// then replaced by the schema default, which is a separate
		// documented issue — see TestTypedClientCannotSetZeroMinReplicas.
		MinReplicas:    1,
		MaxReplicas:    7,
		Concurrency:    16,
		Port:           9090,
		InvocationPath: "/mcp",
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resourceapi.MustParse("250m"),
				corev1.ResourceMemory: resourceapi.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resourceapi.MustParse("1"),
				corev1.ResourceMemory: resourceapi.MustParse("1Gi"),
			},
		},
		Env: []corev1.EnvVar{
			{Name: "LOG_LEVEL", Value: "debug"},
			{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			}},
		},
		EnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "agent-secrets"},
			},
		}},
		Timeout:            metav1.Duration{Duration: 90 * time.Second},
		StartupTimeout:     metav1.Duration{Duration: 45 * time.Second},
		ServiceAccountName: "custom-sa",
	}

	agent := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "roundtrip", Namespace: ns},
		Spec:       want,
	}
	if err := k8sClient.Create(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	var got kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "roundtrip"}, &got); err != nil {
		t.Fatalf("get agent: %v", err)
	}

	// Compare field by field so a failure names the pruned field rather
	// than dumping two large structs.
	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"image", got.Spec.Image, want.Image},
		{"imagePullPolicy", got.Spec.ImagePullPolicy, want.ImagePullPolicy},
		{"runtime", got.Spec.Runtime, want.Runtime},
		{"framework", got.Spec.Framework, want.Framework},
		{"protocol", got.Spec.Protocol, want.Protocol},
		{"mode", got.Spec.Mode, want.Mode},
		{"scaleToZeroAfter", got.Spec.ScaleToZeroAfter, want.ScaleToZeroAfter},
		{"minReplicas", got.Spec.MinReplicas, want.MinReplicas},
		{"maxReplicas", got.Spec.MaxReplicas, want.MaxReplicas},
		{"concurrency", got.Spec.Concurrency, want.Concurrency},
		{"port", got.Spec.Port, want.Port},
		{"invocationPath", got.Spec.InvocationPath, want.InvocationPath},
		{"timeout", got.Spec.Timeout, want.Timeout},
		{"startupTimeout", got.Spec.StartupTimeout, want.StartupTimeout},
		{"serviceAccountName", got.Spec.ServiceAccountName, want.ServiceAccountName},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("spec.%s was not persisted: got %v, want %v — is it missing from the CRD schema?", c.field, c.got, c.want)
		}
	}

	// Composite fields need their own assertions.
	if len(got.Spec.ImagePullSecrets) != 1 || got.Spec.ImagePullSecrets[0].Name != "ghcr-creds" {
		t.Errorf("spec.imagePullSecrets was not persisted: %+v", got.Spec.ImagePullSecrets)
	}
	if len(got.Spec.Env) != 2 {
		t.Errorf("spec.env length = %d, want 2: %+v", len(got.Spec.Env), got.Spec.Env)
	} else {
		if got.Spec.Env[0].Value != "debug" {
			t.Errorf("spec.env[0].value = %q, want debug", got.Spec.Env[0].Value)
		}
		// valueFrom is a nested object; a shallow schema silently drops it.
		if got.Spec.Env[1].ValueFrom == nil || got.Spec.Env[1].ValueFrom.FieldRef == nil {
			t.Errorf("spec.env[1].valueFrom was pruned: %+v", got.Spec.Env[1])
		} else if got.Spec.Env[1].ValueFrom.FieldRef.FieldPath != "metadata.name" {
			t.Errorf("spec.env[1].valueFrom.fieldRef.fieldPath = %q", got.Spec.Env[1].ValueFrom.FieldRef.FieldPath)
		}
	}
	if len(got.Spec.EnvFrom) != 1 || got.Spec.EnvFrom[0].SecretRef == nil {
		t.Errorf("spec.envFrom was not persisted: %+v", got.Spec.EnvFrom)
	}
	if got.Spec.Resources.Requests.Cpu().String() != "250m" {
		t.Errorf("spec.resources.requests.cpu = %v, want 250m", got.Spec.Resources.Requests.Cpu())
	}
	if got.Spec.Resources.Limits.Memory().String() != "1Gi" {
		t.Errorf("spec.resources.limits.memory = %v, want 1Gi", got.Spec.Resources.Limits.Memory())
	}
}

// Same guarantee for ModelSpec.
func TestModelSpecSurvivesRoundTrip(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)

	want := kryptonv1alpha1.ModelSpec{
		Source: kryptonv1alpha1.ModelSource{
			HuggingFace: "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
			File:        "qwen2.5-0.5b-instruct-q4_k_m.gguf",
		},
		Runtime:          kryptonv1alpha1.RuntimeLlamaCpp,
		Image:            "ghcr.io/ggml-org/llama.cpp:server-custom",
		ImagePullPolicy:  corev1.PullIfNotPresent,
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "hf-creds"}},
		Port:             9000,
		Args:             []string{"--ctx-size", "4096", "--threads", "4"},
		MinReplicas:      2,
		MaxReplicas:      3,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceMemory: resourceapi.MustParse("2Gi")},
		},
		Env: []corev1.EnvVar{{Name: "LLAMA_ARG_NO_WARMUP", Value: "1"}},
		EnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "hf-token"},
			},
		}},
		StartupTimeout:     metav1.Duration{Duration: 900 * time.Second},
		ServiceAccountName: "model-sa",
	}

	model := &kryptonv1alpha1.Model{
		ObjectMeta: metav1.ObjectMeta{Name: "roundtrip", Namespace: ns},
		Spec:       want,
	}
	if err := k8sClient.Create(ctx, model); err != nil {
		t.Fatalf("create model: %v", err)
	}

	var got kryptonv1alpha1.Model
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "roundtrip"}, &got); err != nil {
		t.Fatalf("get model: %v", err)
	}

	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"source.huggingface", got.Spec.Source.HuggingFace, want.Source.HuggingFace},
		{"source.file", got.Spec.Source.File, want.Source.File},
		{"runtime", got.Spec.Runtime, want.Runtime},
		{"image", got.Spec.Image, want.Image},
		{"imagePullPolicy", got.Spec.ImagePullPolicy, want.ImagePullPolicy},
		{"port", got.Spec.Port, want.Port},
		{"minReplicas", got.Spec.MinReplicas, want.MinReplicas},
		{"maxReplicas", got.Spec.MaxReplicas, want.MaxReplicas},
		{"startupTimeout", got.Spec.StartupTimeout, want.StartupTimeout},
		{"serviceAccountName", got.Spec.ServiceAccountName, want.ServiceAccountName},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("spec.%s was not persisted: got %v, want %v — is it missing from the CRD schema?", c.field, c.got, c.want)
		}
	}

	if len(got.Spec.Args) != 4 {
		t.Errorf("spec.args = %v, want 4 elements", got.Spec.Args)
	}
	if len(got.Spec.ImagePullSecrets) != 1 {
		t.Errorf("spec.imagePullSecrets was not persisted: %+v", got.Spec.ImagePullSecrets)
	}
	if len(got.Spec.Env) != 1 || got.Spec.Env[0].Name != "LLAMA_ARG_NO_WARMUP" {
		t.Errorf("spec.env was not persisted: %+v", got.Spec.Env)
	}
	if len(got.Spec.EnvFrom) != 1 {
		t.Errorf("spec.envFrom was not persisted: %+v", got.Spec.EnvFrom)
	}
	if got.Spec.Resources.Requests.Memory().String() != "2Gi" {
		t.Errorf("spec.resources.requests.memory = %v, want 2Gi", got.Spec.Resources.Requests.Memory())
	}
}

// The CRD's +kubebuilder:default markers are the ONLY defaulting that runs
// in a real cluster: the chart ships enableWebhooks=false and there is no
// ValidatingWebhookConfiguration, so AgentDefaulter never executes. These
// assertions pin the schema defaults independently of the webhook code.
func TestAgentSchemaDefaults(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)

	// Only the one required field.
	agent := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "defaults", Namespace: ns},
		Spec:       kryptonv1alpha1.AgentSpec{Image: "ghcr.io/org/minimal:latest"},
	}
	if err := k8sClient.Create(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	var got kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "defaults"}, &got); err != nil {
		t.Fatalf("get agent: %v", err)
	}

	// Scalar and string defaults apply as expected, because omitempty
	// genuinely elides their zero values from the request body.
	tests := []struct {
		field string
		got   any
		want  any
	}{
		{"mode", got.Spec.Mode, kryptonv1alpha1.ModeAlwaysOn},
		{"protocol", got.Spec.Protocol, kryptonv1alpha1.ProtocolA2A},
		{"port", got.Spec.Port, int32(8080)},
		{"invocationPath", got.Spec.InvocationPath, "/"},
		{"concurrency", got.Spec.Concurrency, int32(8)},
		{"minReplicas", got.Spec.MinReplicas, int32(1)},
		{"maxReplicas", got.Spec.MaxReplicas, int32(10)},
		{"imagePullPolicy", got.Spec.ImagePullPolicy, corev1.PullIfNotPresent},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("spec.%s defaulted to %v, want %v", tc.field, tc.got, tc.want)
		}
	}

	// The metav1.Duration fields do NOT get their schema defaults here.
	// See TestDurationDefaultsAreDefeatedByTypedClient for why, and for the
	// proof that the CRD itself is correct.
	if got.Spec.Timeout.Duration != 0 {
		t.Logf("note: spec.timeout defaulted to %v — the typed-client "+
			"serialization issue may have been fixed; tighten this test",
			got.Spec.Timeout.Duration)
	}
}

// FINDING (test documents current behaviour, not desired behaviour).
//
// AgentSpec.MinReplicas is `int32` with `json:"minReplicas,omitempty"` and
// `+kubebuilder:default=1`. Those two are in direct conflict for the value
// 0: omitempty elides it from the request body, the API server then sees an
// absent field, and applies the default of 1.
//
// The practical consequence is that the documented serverless workflow
// ("explicitly set to 0 when running in serverless", agent_types.go) is
// impossible through any typed Go client. `kubectl apply` with an explicit
// `minReplicas: 0` in YAML still works, because that JSON carries the 0.
//
// Fixing it means making the field *int32, or dropping the schema default.
// Both are API changes, so this test pins the current behaviour and will
// fail loudly if it is addressed.
func TestTypedClientCannotSetZeroMinReplicas(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)

	agent := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "serverless-zero", Namespace: ns},
		Spec: kryptonv1alpha1.AgentSpec{
			Image:       "ghcr.io/org/a:1",
			Mode:        kryptonv1alpha1.ModeServerless,
			MinReplicas: 0, // what the caller asked for
		},
	}
	if err := k8sClient.Create(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	var got kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "serverless-zero"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Spec.MinReplicas != 1 {
		t.Fatalf("spec.minReplicas = %d; expected the omitempty+default conflict to yield 1. "+
			"If this now returns 0, the field was made a pointer or the default removed — "+
			"delete this test and assert 0 in the round-trip test instead.",
			got.Spec.MinReplicas)
	}

	// Proof that the CRD schema is not at fault: an explicit 0 in the
	// request body (as kubectl would send) is honoured.
	explicit := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "krypton.ai/v1alpha1",
		"kind":       "Agent",
		"metadata":   map[string]any{"name": "serverless-explicit", "namespace": ns},
		"spec": map[string]any{
			"image":       "ghcr.io/org/a:1",
			"mode":        "serverless",
			"minReplicas": int64(0),
		},
	}}
	if err := k8sClient.Create(ctx, explicit); err != nil {
		t.Fatalf("create unstructured: %v", err)
	}
	var readBack kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "serverless-explicit"}, &readBack); err != nil {
		t.Fatalf("get unstructured: %v", err)
	}
	if readBack.Spec.MinReplicas != 0 {
		t.Errorf("an explicit minReplicas:0 came back as %d — the CRD default is "+
			"overriding an explicitly provided value, which would be a schema bug",
			readBack.Spec.MinReplicas)
	}
}

// FINDING (test documents current behaviour, not desired behaviour).
//
// scaleToZeroAfter, timeout and startupTimeout are `metav1.Duration`, which
// is a struct. encoding/json's `omitempty` has no effect on structs, so a
// typed Go client always serializes them — as "0s" when unset. The API
// server therefore sees an explicitly provided value and does not apply the
// CRD defaults (300s / 60s / 30s).
//
// Net effect: an Agent created through the Go API reports
// `scaleToZeroAfter: 0s, timeout: 0s, startupTimeout: 0s`, contradicting
// the documented defaults. `kubectl apply` from YAML is unaffected, because
// the fields are genuinely absent there.
//
// Downstream code mostly defends itself (scaler.isIdle falls back to 5m,
// Activator falls back to DefaultStartupTimeout), but the controller does
// propagate KRYPTON_IDLE_TIMEOUT="0s" to the sidecar, and the values are
// visibly wrong in `kubectl get agent -o yaml`.
//
// Fix would be *metav1.Duration, or a defaulting webhook that is actually
// deployed. Both are outside the test harness.
func TestDurationDefaultsAreDefeatedByTypedClient(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)

	// Path 1: typed client. Durations arrive as "0s" and stay zero.
	typed := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "typed", Namespace: ns},
		Spec:       kryptonv1alpha1.AgentSpec{Image: "ghcr.io/org/a:1"},
	}
	if err := k8sClient.Create(ctx, typed); err != nil {
		t.Fatalf("create typed: %v", err)
	}
	var fromTyped kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "typed"}, &fromTyped); err != nil {
		t.Fatalf("get typed: %v", err)
	}
	for _, c := range []struct {
		field string
		got   time.Duration
	}{
		{"scaleToZeroAfter", fromTyped.Spec.ScaleToZeroAfter.Duration},
		{"timeout", fromTyped.Spec.Timeout.Duration},
		{"startupTimeout", fromTyped.Spec.StartupTimeout.Duration},
	} {
		if c.got != 0 {
			t.Errorf("spec.%s = %v via the typed client; expected 0 because "+
				"omitempty does not elide a metav1.Duration struct. If this is "+
				"now defaulted, the field became a pointer — update this test.",
				c.field, c.got)
		}
	}

	// Path 2: the fields genuinely absent, as kubectl sends them. This
	// proves the CRD defaults themselves are correct and the problem is
	// purely client-side serialization.
	absent := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "krypton.ai/v1alpha1",
		"kind":       "Agent",
		"metadata":   map[string]any{"name": "absent", "namespace": ns},
		"spec":       map[string]any{"image": "ghcr.io/org/a:1"},
	}}
	if err := k8sClient.Create(ctx, absent); err != nil {
		t.Fatalf("create unstructured: %v", err)
	}
	var fromYAML kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "absent"}, &fromYAML); err != nil {
		t.Fatalf("get unstructured: %v", err)
	}
	for _, c := range []struct {
		field string
		got   time.Duration
		want  time.Duration
	}{
		{"scaleToZeroAfter", fromYAML.Spec.ScaleToZeroAfter.Duration, 300 * time.Second},
		{"timeout", fromYAML.Spec.Timeout.Duration, 60 * time.Second},
		{"startupTimeout", fromYAML.Spec.StartupTimeout.Duration, 30 * time.Second},
	} {
		if c.got != c.want {
			t.Errorf("spec.%s = %v when the field is absent, want %v — "+
				"the CRD default is missing or wrong", c.field, c.got, c.want)
		}
	}
}

func TestModelSchemaDefaults(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)

	model := &kryptonv1alpha1.Model{
		ObjectMeta: metav1.ObjectMeta{Name: "defaults", Namespace: ns},
		Spec: kryptonv1alpha1.ModelSpec{
			Source: kryptonv1alpha1.ModelSource{
				HuggingFace: "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
				File:        "qwen2.5-0.5b-instruct-q4_k_m.gguf",
			},
		},
	}
	if err := k8sClient.Create(ctx, model); err != nil {
		t.Fatalf("create model: %v", err)
	}

	var got kryptonv1alpha1.Model
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "defaults"}, &got); err != nil {
		t.Fatalf("get model: %v", err)
	}

	tests := []struct {
		field string
		got   any
		want  any
	}{
		{"runtime", got.Spec.Runtime, kryptonv1alpha1.RuntimeLlamaCpp},
		{"port", got.Spec.Port, int32(8080)},
		{"minReplicas", got.Spec.MinReplicas, int32(1)},
		{"maxReplicas", got.Spec.MaxReplicas, int32(1)},
		{"imagePullPolicy", got.Spec.ImagePullPolicy, corev1.PullIfNotPresent},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("spec.%s defaulted to %v, want %v", tc.field, tc.got, tc.want)
		}
	}

	// ModelSpec.StartupTimeout is metav1.Duration and hits the same
	// omitempty-on-struct issue as the Agent durations, so its 600s default
	// does not apply via the typed client. Verified against an absent field
	// below, which proves the CRD default is correct.
	if got.Spec.StartupTimeout.Duration != 0 {
		t.Logf("note: spec.startupTimeout defaulted to %v via the typed client — "+
			"the serialization issue may be fixed; tighten this test",
			got.Spec.StartupTimeout.Duration)
	}

	absent := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "krypton.ai/v1alpha1",
		"kind":       "Model",
		"metadata":   map[string]any{"name": "defaults-absent", "namespace": ns},
		"spec": map[string]any{
			"source": map[string]any{
				"huggingface": "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
				"file":        "qwen2.5-0.5b-instruct-q4_k_m.gguf",
			},
		},
	}}
	if err := k8sClient.Create(ctx, absent); err != nil {
		t.Fatalf("create unstructured model: %v", err)
	}
	var fromYAML kryptonv1alpha1.Model
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "defaults-absent"}, &fromYAML); err != nil {
		t.Fatalf("get unstructured model: %v", err)
	}
	if want := 600 * time.Second; fromYAML.Spec.StartupTimeout.Duration != want {
		t.Errorf("spec.startupTimeout = %v when absent, want %v — the CRD default is missing or wrong",
			fromYAML.Spec.StartupTimeout.Duration, want)
	}
}

// Schema-level validation the API server enforces without any webhook.
func TestAgentSchemaValidation(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)

	tests := []struct {
		name    string
		spec    kryptonv1alpha1.AgentSpec
		wantErr bool
	}{
		{
			name:    "valid minimal",
			spec:    kryptonv1alpha1.AgentSpec{Image: "ghcr.io/org/a:1"},
			wantErr: false,
		},
		{
			// +kubebuilder:validation:MinLength=1
			name:    "empty image rejected",
			spec:    kryptonv1alpha1.AgentSpec{Image: ""},
			wantErr: true,
		},
		{
			// +kubebuilder:validation:Enum=serverless;always-on
			name:    "unknown mode rejected",
			spec:    kryptonv1alpha1.AgentSpec{Image: "ghcr.io/org/a:1", Mode: "sometimes"},
			wantErr: true,
		},
		{
			// +kubebuilder:validation:Enum=a2a;mcp;http
			name:    "unknown protocol rejected",
			spec:    kryptonv1alpha1.AgentSpec{Image: "ghcr.io/org/a:1", Protocol: "grpc"},
			wantErr: true,
		},
		{
			// +kubebuilder:validation:Maximum=65535
			name:    "port above 65535 rejected",
			spec:    kryptonv1alpha1.AgentSpec{Image: "ghcr.io/org/a:1", Port: 70000},
			wantErr: true,
		},
		{
			// +kubebuilder:validation:Minimum=1
			name:    "concurrency below 1 rejected",
			spec:    kryptonv1alpha1.AgentSpec{Image: "ghcr.io/org/a:1", Concurrency: -1},
			wantErr: true,
		},
		{
			// +kubebuilder:validation:Minimum=0 — zero is legal for serverless
			name:    "minReplicas zero accepted",
			spec:    kryptonv1alpha1.AgentSpec{Image: "ghcr.io/org/a:1", Mode: kryptonv1alpha1.ModeServerless, MinReplicas: 0},
			wantErr: false,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agent := &kryptonv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "validate-" + string(rune('a'+i)),
					Namespace: ns,
				},
				Spec: tc.spec,
			}
			err := k8sClient.Create(ctx, agent)
			if tc.wantErr && err == nil {
				t.Fatal("expected the API server to reject this spec, but it was accepted")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected the spec to be accepted, got: %v", err)
			}
			if tc.wantErr && !apierrors.IsInvalid(err) && !apierrors.IsBadRequest(err) {
				t.Errorf("error = %v, want Invalid or BadRequest", err)
			}
		})
	}
}

// Documents a real gap rather than asserting desired behaviour: the
// "always-on implies minReplicas >= 1" rule lives only in the webhook
// validator (api/v1alpha1/agent_webhook.go validateSpec), and the webhook
// is not deployed. OpenAPI cannot express a cross-field constraint, so the
// API server accepts the contradictory combination.
//
// If webhooks get enabled, or the rule moves to a CEL
// x-kubernetes-validations rule in the CRD, this test will start failing
// and should be inverted to assert rejection.
func TestAlwaysOnWithZeroMinReplicasIsNotRejectedBySchema(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)

	// Send minReplicas: 0 explicitly, as YAML would — the typed client
	// would elide it and get the default of 1 instead (see
	// TestTypedClientCannotSetZeroMinReplicas).
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "krypton.ai/v1alpha1",
		"kind":       "Agent",
		"metadata":   map[string]any{"name": "contradictory", "namespace": ns},
		"spec": map[string]any{
			"image":       "ghcr.io/org/a:1",
			"mode":        "always-on",
			"minReplicas": int64(0),
		},
	}}
	if err := k8sClient.Create(ctx, obj); err != nil {
		t.Fatalf("unexpected rejection — if a CEL x-kubernetes-validations rule "+
			"or a deployed webhook now enforces this, invert this test to expect "+
			"an error: %v", err)
	}

	// The contradictory state persisted.
	var stored kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "contradictory"}, &stored); err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Spec.Mode != kryptonv1alpha1.ModeAlwaysOn || stored.Spec.MinReplicas != 0 {
		t.Fatalf("expected always-on with minReplicas=0 to persist, got mode=%q minReplicas=%d",
			stored.Spec.Mode, stored.Spec.MinReplicas)
	}

	// The webhook validator WOULD have caught it — so the gap is that the
	// webhook isn't deployed (values.yaml sets enableWebhooks: false and
	// there is no ValidatingWebhookConfiguration), not that the rule is
	// missing. Validate a locally built object, not the server round-trip.
	local := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "contradictory", Namespace: ns},
		Spec: kryptonv1alpha1.AgentSpec{
			Image:       "ghcr.io/org/a:1",
			Mode:        kryptonv1alpha1.ModeAlwaysOn,
			MinReplicas: 0,
			Concurrency: 8,
			Port:        8080,
			MaxReplicas: 10,
		},
	}
	if _, vErr := (&kryptonv1alpha1.AgentValidator{}).ValidateCreate(ctx, local); vErr == nil {
		t.Error("webhook ValidateCreate should reject always-on with minReplicas=0")
	}
}

// The status subresource must be separate: a plain Update on the main
// resource cannot write status, and a status write cannot change spec.
func TestStatusSubresourceIsSeparate(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	key := types.NamespacedName{Namespace: ns, Name: "subresource"}

	agent := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: ns},
		Spec:       kryptonv1alpha1.AgentSpec{Image: "ghcr.io/org/a:1"},
	}
	if err := k8sClient.Create(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Attempt to set status through the main resource. The API server
	// silently drops it because status is a subresource.
	//
	// Both writes retry: the controller is reconciling this object
	// concurrently, so a resourceVersion conflict is expected and is itself
	// part of what this tier exercises (the fake client never produces one).
	eventually(t, "spec Update carrying a status change to be accepted", func() error {
		var fetched kryptonv1alpha1.Agent
		if err := k8sClient.Get(ctx, key, &fetched); err != nil {
			return err
		}
		fetched.Status.Phase = kryptonv1alpha1.PhaseFailed
		fetched.Status.Replicas = 99
		return k8sClient.Update(ctx, &fetched)
	})

	var afterSpecWrite kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, key, &afterSpecWrite); err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if afterSpecWrite.Status.Replicas == 99 {
		t.Error("a spec Update wrote status.replicas — status is not registered as a subresource")
	}

	// The reverse: a status write must not clobber spec.
	eventually(t, "status Update carrying a spec change to be accepted", func() error {
		var fetched kryptonv1alpha1.Agent
		if err := k8sClient.Get(ctx, key, &fetched); err != nil {
			return err
		}
		fetched.Status.Phase = kryptonv1alpha1.PhaseReady
		fetched.Spec.Image = "ghcr.io/org/SHOULD-NOT-PERSIST:1"
		return k8sClient.Status().Update(ctx, &fetched)
	})

	var afterStatusWrite kryptonv1alpha1.Agent
	if err := k8sClient.Get(ctx, key, &afterStatusWrite); err != nil {
		t.Fatalf("get after status update: %v", err)
	}
	if afterStatusWrite.Spec.Image == "ghcr.io/org/SHOULD-NOT-PERSIST:1" {
		t.Error("a status Update wrote spec.image")
	}
	if afterStatusWrite.Status.Phase != kryptonv1alpha1.PhaseReady {
		t.Errorf("status.phase = %q, want Ready", afterStatusWrite.Status.Phase)
	}
}

// Pruning is the mechanism that makes a stale CRD schema dangerous: a
// field the schema doesn't know about is dropped silently, with no error to
// the caller. This demonstrates it directly by writing an unstructured
// Agent carrying a field the schema has never heard of.
//
// It's the proof that TestAgentSpecSurvivesRoundTrip is load-bearing — if
// pruning were not silent, a missing schema property would surface as an
// API error instead of as data loss.
func TestUnknownFieldsArePrunedSilently(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "krypton.ai/v1alpha1",
		"kind":       "Agent",
		"metadata": map[string]any{
			"name":      "pruned",
			"namespace": ns,
		},
		"spec": map[string]any{
			"image": "ghcr.io/org/a:1",
			// Not present in AgentSpec or in the CRD schema.
			"totallyMadeUpField": "should not survive",
		},
	}}

	// The write succeeds — no error, no warning about the unknown field.
	if err := k8sClient.Create(ctx, obj); err != nil {
		t.Fatalf("create unstructured agent: %v", err)
	}

	var readBack unstructured.Unstructured
	readBack.SetGroupVersionKind(obj.GroupVersionKind())
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "pruned"}, &readBack); err != nil {
		t.Fatalf("get: %v", err)
	}

	spec, ok := readBack.Object["spec"].(map[string]any)
	if !ok {
		t.Fatalf("spec is not an object: %#v", readBack.Object["spec"])
	}
	if _, present := spec["totallyMadeUpField"]; present {
		t.Error("unknown field survived the write; the CRD is missing " +
			"x-kubernetes-preserve-unknown-fields=false or structural pruning is off")
	}
	// The known field is untouched, confirming only the unknown one was dropped.
	if spec["image"] != "ghcr.io/org/a:1" {
		t.Errorf("spec.image = %v, want ghcr.io/org/a:1", spec["image"])
	}
}
