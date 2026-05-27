/*
Copyright 2026 Krypton Authors.
*/

package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

func sampleModel(name, ns string) *kryptonv1alpha1.Model {
	return &kryptonv1alpha1.Model{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: kryptonv1alpha1.ModelSpec{
			Source: kryptonv1alpha1.ModelSource{
				HuggingFace: "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
				File:        "qwen2.5-0.5b-instruct-q4_k_m.gguf",
			},
			Runtime: kryptonv1alpha1.RuntimeLlamaCpp,
			Port:    8080,
		},
	}
}

func newGatewayWithModels(t *testing.T, objs ...kryptonv1alpha1.Model) (*Gateway, *httptest.Server, *recordingUpstream) {
	t.Helper()
	s := testScheme(t)
	b := fake.NewClientBuilder().WithScheme(s)
	for i := range objs {
		b = b.WithObjects(&objs[i])
	}
	cli := b.Build()

	up := &recordingUpstream{}
	ts := httptest.NewServer(up)
	t.Cleanup(ts.Close)
	tsURL, _ := url.Parse(ts.URL)

	gw := &Gateway{
		ModelResolver:  &ModelResolver{Client: cli},
		OverrideTarget: func(*url.URL) *url.URL { return tsURL },
	}
	return gw, ts, up
}

type recordingUpstream struct {
	path string
	body []byte
}

func (r *recordingUpstream) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.path = req.URL.Path
	r.body, _ = io.ReadAll(req.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion"}`))
}

func TestListModels(t *testing.T) {
	gw, _, _ := newGatewayWithModels(t, *sampleModel("qwen", "models"), *sampleModel("llama", "models"))
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var out openaiModelList
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Object != "list" || len(out.Data) != 2 {
		t.Fatalf("unexpected list: %+v", out)
	}
	ids := map[string]bool{out.Data[0].ID: true, out.Data[1].ID: true}
	if !ids["qwen"] || !ids["llama"] {
		t.Errorf("missing expected ids: %+v", ids)
	}
}

func TestGetModel(t *testing.T) {
	gw, _, _ := newGatewayWithModels(t, *sampleModel("qwen", "models"))
	req := httptest.NewRequest(http.MethodGet, "/v1/models/qwen", nil)
	rr := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var m openaiModel
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.ID != "qwen" || m.Namespace != "models" {
		t.Errorf("unexpected card: %+v", m)
	}
}

func TestGetModelNotFound(t *testing.T) {
	gw, _, _ := newGatewayWithModels(t, *sampleModel("qwen", "models"))
	req := httptest.NewRequest(http.MethodGet, "/v1/models/missing", nil)
	rr := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestChatCompletionsRoutesByBodyModel(t *testing.T) {
	gw, _, up := newGatewayWithModels(t, *sampleModel("qwen", "models"))
	body := `{"model":"qwen","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if up.path != "/v1/chat/completions" {
		t.Errorf("upstream path = %q, want /v1/chat/completions", up.path)
	}
	if string(up.body) != body {
		t.Errorf("upstream body = %q, want %q", up.body, body)
	}
}

func TestChatCompletionsUnknownModel(t *testing.T) {
	gw, _, _ := newGatewayWithModels(t, *sampleModel("qwen", "models"))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"nope","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestChatCompletionsMissingModel(t *testing.T) {
	gw, _, _ := newGatewayWithModels(t, *sampleModel("qwen", "models"))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestModelRoutesDisabledWhenResolverNil(t *testing.T) {
	gw := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (route not registered)", rr.Code)
	}
}
