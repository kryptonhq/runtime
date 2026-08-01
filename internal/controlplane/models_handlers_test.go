/*
Copyright 2026 Krypton Authors.
*/

package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

// A plain error carrying no status — writeAPIErr must default it to 500.
var errUnexpected = errors.New("database on fire")

func TestGetModelStatus(t *testing.T) {
	m := newSampleModel("qwen", "models")
	m.Status = kryptonv1alpha1.ModelStatus{
		Phase:              kryptonv1alpha1.ModelPhaseReady,
		Replicas:           2,
		ReadyReplicas:      2,
		URL:                "http://qwen.models.svc:8080",
		ObservedGeneration: 7,
		Conditions: []metav1.Condition{{
			Type:   kryptonv1alpha1.ConditionAvailable,
			Status: metav1.ConditionTrue,
			Reason: "MinimumReplicasAvailable",
		}},
	}

	api := &API{Client: testClient(t, m)}
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/v1/models/models/qwen/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// The endpoint returns the bare status object, not a wrapped view.
	var got kryptonv1alpha1.ModelStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if got.Phase != kryptonv1alpha1.ModelPhaseReady {
		t.Errorf("Phase = %q, want Ready", got.Phase)
	}
	if got.ReadyReplicas != 2 || got.Replicas != 2 {
		t.Errorf("replicas = %d/%d, want 2/2", got.ReadyReplicas, got.Replicas)
	}
	if got.URL != "http://qwen.models.svc:8080" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.ObservedGeneration != 7 {
		t.Errorf("ObservedGeneration = %d, want 7", got.ObservedGeneration)
	}
	if len(got.Conditions) != 1 {
		t.Fatalf("conditions = %d, want 1", len(got.Conditions))
	}
}

func TestGetModelStatusNotFound(t *testing.T) {
	api := &API{Client: testClient(t)}
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/v1/models/models/ghost/status", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestGetModelNotFound(t *testing.T) {
	api := &API{Client: testClient(t)}
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/v1/models/models/ghost", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// lessModel drives ?sort= for /v1/models. Every branch has a documented
// tiebreak on name, which is what keeps pagination stable across requests.
func TestLessModel(t *testing.T) {
	mk := func(name, ns string, phase kryptonv1alpha1.ModelPhase, replicas int32, runtime kryptonv1alpha1.ModelRuntime, hf string) *ModelView {
		return &ModelView{
			Name:      name,
			Namespace: ns,
			Spec: kryptonv1alpha1.ModelSpec{
				Runtime: runtime,
				Source:  kryptonv1alpha1.ModelSource{HuggingFace: hf},
			},
			Status: kryptonv1alpha1.ModelStatus{Phase: phase, Replicas: replicas},
		}
	}

	tests := []struct {
		name  string
		a, b  *ModelView
		field string
		desc  bool
		want  bool
	}{
		{
			name:  "name ascending",
			a:     mk("alpha", "models", "", 0, "", ""),
			b:     mk("beta", "models", "", 0, "", ""),
			field: "name",
			want:  true,
		},
		{
			name:  "name descending",
			a:     mk("alpha", "models", "", 0, "", ""),
			b:     mk("beta", "models", "", 0, "", ""),
			field: "name",
			desc:  true,
			want:  false,
		},
		{
			// Same name in two namespaces: namespace breaks the tie.
			name:  "name tie falls back to namespace",
			a:     mk("qwen", "a-ns", "", 0, "", ""),
			b:     mk("qwen", "b-ns", "", 0, "", ""),
			field: "name",
			want:  true,
		},
		{
			name:  "namespace ascending",
			a:     mk("z", "a-ns", "", 0, "", ""),
			b:     mk("a", "b-ns", "", 0, "", ""),
			field: "namespace",
			want:  true,
		},
		{
			name:  "namespace tie falls back to name",
			a:     mk("alpha", "models", "", 0, "", ""),
			b:     mk("beta", "models", "", 0, "", ""),
			field: "namespace",
			want:  true,
		},
		{
			// Phases sort lexically, not by lifecycle order: Failed < Pending < Ready.
			name:  "phase ascending is lexical",
			a:     mk("a", "models", kryptonv1alpha1.ModelPhaseFailed, 0, "", ""),
			b:     mk("b", "models", kryptonv1alpha1.ModelPhaseReady, 0, "", ""),
			field: "phase",
			want:  true,
		},
		{
			name:  "phase tie falls back to name",
			a:     mk("alpha", "models", kryptonv1alpha1.ModelPhaseReady, 0, "", ""),
			b:     mk("beta", "models", kryptonv1alpha1.ModelPhaseReady, 0, "", ""),
			field: "phase",
			want:  true,
		},
		{
			name:  "replicas ascending",
			a:     mk("a", "models", "", 1, "", ""),
			b:     mk("b", "models", "", 3, "", ""),
			field: "replicas",
			want:  true,
		},
		{
			name:  "replicas descending",
			a:     mk("a", "models", "", 1, "", ""),
			b:     mk("b", "models", "", 3, "", ""),
			field: "replicas",
			desc:  true,
			want:  false,
		},
		{
			name:  "replicas tie falls back to name",
			a:     mk("alpha", "models", "", 2, "", ""),
			b:     mk("beta", "models", "", 2, "", ""),
			field: "replicas",
			want:  true,
		},
		{
			name:  "runtime ascending",
			a:     mk("a", "models", "", 0, "llama.cpp", ""),
			b:     mk("b", "models", "", 0, "vllm", ""),
			field: "runtime",
			want:  true,
		},
		{
			name:  "runtime tie falls back to name",
			a:     mk("alpha", "models", "", 0, "llama.cpp", ""),
			b:     mk("beta", "models", "", 0, "llama.cpp", ""),
			field: "runtime",
			want:  true,
		},
		{
			name:  "source ascending",
			a:     mk("a", "models", "", 0, "", "Qwen/Qwen2.5"),
			b:     mk("b", "models", "", 0, "", "TinyLlama/TinyLlama"),
			field: "source",
			want:  true,
		},
		{
			name:  "source tie falls back to name",
			a:     mk("alpha", "models", "", 0, "", "Qwen/Qwen2.5"),
			b:     mk("beta", "models", "", 0, "", "Qwen/Qwen2.5"),
			field: "source",
			want:  true,
		},
		{
			// Unrecognised sort fields must not error; they fall through to
			// name so the API stays forgiving about query params.
			name:  "unknown field falls through to name",
			a:     mk("alpha", "models", "", 0, "", ""),
			b:     mk("beta", "models", "", 0, "", ""),
			field: "nonsense",
			want:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lessModel(tc.a, tc.b, tc.field, tc.desc); got != tc.want {
				t.Errorf("lessModel(%s, desc=%v) = %v, want %v", tc.field, tc.desc, got, tc.want)
			}
		})
	}
}

// Same coverage for the agent comparator's less-travelled branches.
func TestLessAgent(t *testing.T) {
	mk := func(name, ns string, phase kryptonv1alpha1.AgentPhase, replicas int32, image string) *AgentView {
		return &AgentView{
			Name:      name,
			Namespace: ns,
			Spec:      kryptonv1alpha1.AgentSpec{Image: image},
			Status:    kryptonv1alpha1.AgentStatus{Phase: phase, Replicas: replicas},
		}
	}

	tests := []struct {
		name  string
		a, b  *AgentView
		field string
		desc  bool
		want  bool
	}{
		{
			name:  "image ascending",
			a:     mk("a", "agents", "", 0, "ghcr.io/a:1"),
			b:     mk("b", "agents", "", 0, "ghcr.io/b:1"),
			field: "image",
			want:  true,
		},
		{
			name:  "image tie falls back to name",
			a:     mk("alpha", "agents", "", 0, "ghcr.io/same:1"),
			b:     mk("beta", "agents", "", 0, "ghcr.io/same:1"),
			field: "image",
			want:  true,
		},
		{
			// "Ready" > "Pending" lexically, so descending order puts Ready
			// first — meaning Pending does NOT sort before Ready.
			name:  "phase descending",
			a:     mk("a", "agents", kryptonv1alpha1.PhasePending, 0, ""),
			b:     mk("b", "agents", kryptonv1alpha1.PhaseReady, 0, ""),
			field: "phase",
			desc:  true,
			want:  false,
		},
		{
			name:  "replicas tie falls back to name",
			a:     mk("alpha", "agents", "", 3, ""),
			b:     mk("beta", "agents", "", 3, ""),
			field: "replicas",
			want:  true,
		},
		{
			name:  "namespace tie falls back to name",
			a:     mk("alpha", "agents", "", 0, ""),
			b:     mk("beta", "agents", "", 0, ""),
			field: "namespace",
			want:  true,
		},
		{
			name:  "name tie falls back to namespace",
			a:     mk("same", "a-ns", "", 0, ""),
			b:     mk("same", "b-ns", "", 0, ""),
			field: "name",
			want:  true,
		},
		{
			name:  "unknown field falls through to name",
			a:     mk("alpha", "agents", "", 0, ""),
			b:     mk("beta", "agents", "", 0, ""),
			field: "whatever",
			want:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lessAgent(tc.a, tc.b, tc.field, tc.desc); got != tc.want {
				t.Errorf("lessAgent(%s, desc=%v) = %v, want %v", tc.field, tc.desc, got, tc.want)
			}
		})
	}
}

func TestMatchesModelSearch(t *testing.T) {
	v := &ModelView{
		Name:      "qwen",
		Namespace: "models",
		Spec: kryptonv1alpha1.ModelSpec{
			Runtime: kryptonv1alpha1.RuntimeLlamaCpp,
			Source: kryptonv1alpha1.ModelSource{
				HuggingFace: "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
				File:        "qwen2.5-0.5b-instruct-q4_k_m.gguf",
			},
		},
	}
	// Needles arrive already lowercased from the handler.
	matches := []string{"qwen", "models", "instruct-gguf", "q4_k_m", "llama.cpp"}
	for _, needle := range matches {
		if !matchesModelSearch(v, needle) {
			t.Errorf("matchesModelSearch(%q) = false, want true", needle)
		}
	}
	for _, needle := range []string{"mistral", "vllm", "zzz"} {
		if matchesModelSearch(v, needle) {
			t.Errorf("matchesModelSearch(%q) = true, want false", needle)
		}
	}
}

func TestAtoiOr(t *testing.T) {
	tests := []struct {
		in       string
		fallback int
		want     int
	}{
		{"", 20, 20},
		{"5", 20, 5},
		{"notanumber", 20, 20},
		{"-3", 1, -3}, // parsed fine; the handlers clamp, not this helper
		{"0", 1, 0},
	}
	for _, tc := range tests {
		if got := atoiOr(tc.in, tc.fallback); got != tc.want {
			t.Errorf("atoiOr(%q, %d) = %d, want %d", tc.in, tc.fallback, got, tc.want)
		}
	}
}

// apiError carries its own HTTP status; writeAPIErr must honour it rather
// than flattening everything to 500.
func TestWriteAPIErrMapsStatuses(t *testing.T) {
	t.Run("bad request from missing path values", func(t *testing.T) {
		rec := httptest.NewRecorder()
		writeAPIErr(rec, errBadRequest("namespace and name are required"))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
	})

	t.Run("unknown error becomes 500", func(t *testing.T) {
		rec := httptest.NewRecorder()
		writeAPIErr(rec, errUnexpected)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("code = %d, want 500", rec.Code)
		}
	})
}

func TestAPIErrorImplementsError(t *testing.T) {
	err := apiError{status: http.StatusTeapot, msg: "short and stout"}
	if err.Error() != "short and stout" {
		t.Errorf("Error() = %q", err.Error())
	}
}
