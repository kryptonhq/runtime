/*
Copyright 2026 Krypton Authors.
*/

package controlplane

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

func testClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if err := kryptonv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("krypton scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func newSampleAgent(name, ns string) *kryptonv1alpha1.Agent {
	return &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID("uid-" + name)},
		Spec: kryptonv1alpha1.AgentSpec{
			Image: "ghcr.io/org/" + name + ":latest",
			Mode:  kryptonv1alpha1.ModeServerless,
			Port:  8080,
		},
		Status: kryptonv1alpha1.AgentStatus{
			Phase:    kryptonv1alpha1.PhaseReady,
			Replicas: 1,
		},
	}
}

type listResp struct {
	Items    []AgentView `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
	Total    int         `json:"total"`
}

func decodeList(t *testing.T, body io.Reader) listResp {
	t.Helper()
	var r listResp
	if err := json.NewDecoder(body).Decode(&r); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return r
}

func TestListAgents(t *testing.T) {
	a := newSampleAgent("travel", "agents")
	b := newSampleAgent("billing", "agents")
	c := newSampleAgent("search", "other")
	api := &API{Client: testClient(t, a, b, c)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("GET /v1/agents: err=%v code=%v", err, resp.StatusCode)
	}
	defer resp.Body.Close()
	got := decodeList(t, resp.Body)
	if got.Total != 3 || len(got.Items) != 3 {
		t.Fatalf("got total=%d items=%d, want 3/3", got.Total, len(got.Items))
	}
	// Default sort is by name asc.
	if got.Items[0].Name != "billing" {
		t.Errorf("first item = %q, want billing (alphabetical)", got.Items[0].Name)
	}
}

func TestListAgentsFiltersByNamespace(t *testing.T) {
	a := newSampleAgent("travel", "agents")
	b := newSampleAgent("search", "other")
	api := &API{Client: testClient(t, a, b)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents?namespace=agents")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	got := decodeList(t, resp.Body)
	if got.Total != 1 || got.Items[0].Name != "travel" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestListAgentsFiltersByProtocol(t *testing.T) {
	a := newSampleAgent("travel", "agents")
	a.Spec.Protocol = kryptonv1alpha1.ProtocolA2A
	b := newSampleAgent("hello", "agents")
	b.Spec.Protocol = kryptonv1alpha1.ProtocolMCP
	api := &API{Client: testClient(t, a, b)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents?protocol=mcp")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	got := decodeList(t, resp.Body)
	if got.Total != 1 || got.Items[0].Name != "hello" {
		t.Errorf("protocol filter wrong: %+v", got)
	}
}

func TestListAgentsSearchMatchesNameNamespaceImage(t *testing.T) {
	a := newSampleAgent("travel", "agents")
	b := newSampleAgent("billing", "finance")
	c := newSampleAgent("misc", "kube-system")
	c.Spec.Image = "ghcr.io/special/runner:1"
	api := &API{Client: testClient(t, a, b, c)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	cases := []struct {
		query    string
		wantName string
	}{
		{"trav", "travel"},     // name match
		{"finance", "billing"}, // namespace match
		{"special", "misc"},    // image match
		{"TRAVEL", "travel"},   // case-insensitive
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/v1/agents?q=" + tc.query)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			got := decodeList(t, resp.Body)
			if got.Total != 1 || got.Items[0].Name != tc.wantName {
				t.Errorf("q=%q → %+v, want a single hit for %q", tc.query, got, tc.wantName)
			}
		})
	}
}

func TestListAgentsSort(t *testing.T) {
	a := newSampleAgent("alpha", "agents")
	a.Status.Replicas = 3
	b := newSampleAgent("bravo", "agents")
	b.Status.Replicas = 1
	c := newSampleAgent("charlie", "agents")
	c.Status.Replicas = 2
	api := &API{Client: testClient(t, a, b, c)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	cases := []struct {
		url       string
		wantFirst string
	}{
		{"?sort=name", "alpha"},
		{"?sort=name&order=desc", "charlie"},
		{"?sort=replicas", "bravo"},
		{"?sort=replicas&order=desc", "alpha"},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/v1/agents" + tc.url)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			got := decodeList(t, resp.Body)
			if got.Items[0].Name != tc.wantFirst {
				t.Errorf("%s: first = %q, want %q", tc.url, got.Items[0].Name, tc.wantFirst)
			}
		})
	}
}

func TestListAgentsPagination(t *testing.T) {
	objs := []client.Object{}
	for i := 0; i < 12; i++ {
		objs = append(objs, newSampleAgent(fmt.Sprintf("agent-%02d", i), "agents"))
	}
	api := &API{Client: testClient(t, objs...)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	// Page 1
	resp, err := http.Get(srv.URL + "/v1/agents?pageSize=5&page=1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	page1 := decodeList(t, resp.Body)
	if page1.Total != 12 || page1.PageSize != 5 || page1.Page != 1 || len(page1.Items) != 5 {
		t.Errorf("page1 = %+v", page1)
	}
	if page1.Items[0].Name != "agent-00" {
		t.Errorf("page1 first = %q, want agent-00", page1.Items[0].Name)
	}

	// Page 3 (only 2 left)
	resp2, err := http.Get(srv.URL + "/v1/agents?pageSize=5&page=3")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp2.Body.Close()
	page3 := decodeList(t, resp2.Body)
	if len(page3.Items) != 2 || page3.Items[0].Name != "agent-10" {
		t.Errorf("page3 = %+v", page3)
	}

	// Out-of-range page
	resp3, err := http.Get(srv.URL + "/v1/agents?pageSize=5&page=99")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp3.Body.Close()
	pageX := decodeList(t, resp3.Body)
	if len(pageX.Items) != 0 || pageX.Total != 12 {
		t.Errorf("oor page = %+v", pageX)
	}
}

func TestGetAgent(t *testing.T) {
	a := newSampleAgent("travel", "agents")
	api := &API{Client: testClient(t, a)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents/agents/travel")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("GET: err=%v code=%v", err, resp.StatusCode)
	}
	defer resp.Body.Close()
	var got AgentView
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "travel" || got.Spec.Port != 8080 {
		t.Fatalf("unexpected view: %+v", got)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	api := &API{Client: testClient(t)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents/agents/missing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetAgentStatus(t *testing.T) {
	a := newSampleAgent("travel", "agents")
	api := &API{Client: testClient(t, a)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents/agents/travel/status")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("GET status: err=%v code=%v", err, resp.StatusCode)
	}
	defer resp.Body.Close()
	var got kryptonv1alpha1.AgentStatus
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Phase != kryptonv1alpha1.PhaseReady {
		t.Errorf("phase = %q, want Ready", got.Phase)
	}
}

func TestHealthEndpoint(t *testing.T) {
	api := &API{Client: testClient(t)}
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil || resp.StatusCode != 200 {
			t.Errorf("%s: err=%v code=%v", path, err, resp.StatusCode)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
}
