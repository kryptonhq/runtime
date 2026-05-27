/*
Copyright 2026 Krypton Authors.
*/

package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

func testScheme(t *testing.T) *runtime.Scheme {
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

func sampleAgent() *kryptonv1alpha1.Agent {
	return &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "travel", Namespace: "agents"},
		Spec: kryptonv1alpha1.AgentSpec{
			Image: "img:1",
			Port:  8080,
			Mode:  kryptonv1alpha1.ModeServerless,
		},
	}
}

func readyEndpoints(name, ns string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
		}},
	}
}

func emptyEndpoints(name, ns string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Subsets:    nil,
	}
}

func newGateway(t *testing.T, upstream http.HandlerFunc, objs ...client.Object) (*Gateway, *httptest.Server, *Activator) {
	t.Helper()
	upstreamSrv := httptest.NewServer(upstream)
	t.Cleanup(upstreamSrv.Close)
	upstreamURL, _ := url.Parse(upstreamSrv.URL)

	cli := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithStatusSubresource(&kryptonv1alpha1.Agent{}).
		WithObjects(objs...).
		Build()

	a := &Activator{
		Client:                cli,
		MaxBufferPerAgent:     2,
		PollInterval:          5 * time.Millisecond,
		DefaultStartupTimeout: 500 * time.Millisecond,
	}
	g := &Gateway{
		Activator: a,
		// Tests don't have real cluster DNS — redirect to the upstream server.
		OverrideTarget: func(_ *url.URL) *url.URL { return upstreamURL },
	}
	return g, upstreamSrv, a
}

func TestForwardsWhenEndpointsReady(t *testing.T) {
	called := false
	gw, _, _ := newGateway(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/a2a" {
			t.Errorf("upstream got path %q, want /a2a", r.URL.Path)
		}
		fmt.Fprint(w, "pong")
	}, sampleAgent(), readyEndpoints("travel", "agents"))
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents/agents/travel/a2a")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("body = %q", body)
	}
	if !called {
		t.Fatal("upstream not called")
	}
}

func TestUnknownAgent(t *testing.T) {
	gw, _, _ := newGateway(t, func(http.ResponseWriter, *http.Request) {})
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents/agents/missing/invocations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestColdStartScalesAgentAndWaits(t *testing.T) {
	gw, _, a := newGateway(
		t,
		func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") },
		sampleAgent(), emptyEndpoints("travel", "agents"),
	)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	// Simulate "pod came up" after a small delay by updating the
	// Endpoints object in-place. The fake client requires Get-then-Update
	// so the resource version round-trips correctly.
	go func() {
		time.Sleep(50 * time.Millisecond)
		var eps corev1.Endpoints
		if err := a.Client.Get(context.Background(), types.NamespacedName{Namespace: "agents", Name: "travel"}, &eps); err != nil {
			t.Errorf("background Get endpoints: %v", err)
			return
		}
		eps.Subsets = []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}}
		if err := a.Client.Update(context.Background(), &eps); err != nil {
			t.Errorf("background Update endpoints: %v", err)
		}
	}()

	resp, err := http.Get(srv.URL + "/v1/agents/agents/travel/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	// Verify the activator patched status.desiredReplicas.
	var got kryptonv1alpha1.Agent
	if err := a.Client.Get(context.Background(), types.NamespacedName{Namespace: "agents", Name: "travel"}, &got); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status.DesiredReplicas != 1 {
		t.Errorf("desiredReplicas = %d, want 1", got.Status.DesiredReplicas)
	}
}

func TestColdStartTimeoutReturns504(t *testing.T) {
	gw, _, a := newGateway(
		t,
		func(http.ResponseWriter, *http.Request) {},
		sampleAgent(), emptyEndpoints("travel", "agents"),
	)
	a.DefaultStartupTimeout = 30 * time.Millisecond
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents/agents/travel/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504", resp.StatusCode)
	}
}

func TestBufferFullReturns503WithRetryAfter(t *testing.T) {
	// MaxBufferPerAgent = 2; the third concurrent cold-start gets rejected.
	gw, _, a := newGateway(
		t,
		func(http.ResponseWriter, *http.Request) {},
		sampleAgent(), emptyEndpoints("travel", "agents"),
	)
	a.DefaultStartupTimeout = 200 * time.Millisecond
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	var (
		wg         sync.WaitGroup
		codes      [3]int
		retryAfter [3]string
	)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/v1/agents/agents/travel/")
			if err != nil {
				return
			}
			codes[i] = resp.StatusCode
			retryAfter[i] = resp.Header.Get("Retry-After")
			resp.Body.Close()
		}(i)
		// Stagger slightly so requests register sequentially.
		time.Sleep(5 * time.Millisecond)
	}
	wg.Wait()

	// Expect exactly one 503 with Retry-After (the third, after the buffer fills).
	var n503 int
	for i, c := range codes {
		if c == http.StatusServiceUnavailable {
			n503++
			if retryAfter[i] == "" {
				t.Errorf("503 #%d missing Retry-After", i)
			}
		}
	}
	if n503 != 1 {
		t.Errorf("got %d 503s, want 1; codes=%v", n503, codes)
	}
}

func TestStreamingPassThrough(t *testing.T) {
	// Upstream emits 3 chunks with a delay between them; gateway must flush
	// them as they arrive (FlushInterval = -1), not buffer to EOF.
	gw, _, _ := newGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: %d\n\n", i)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}, sampleAgent(), readyEndpoints("travel", "agents"))
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/agents/agents/travel/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	// Read first chunk, assert it arrives before the upstream finishes
	// (i.e. << 30ms total elapsed).
	buf := make([]byte, 64)
	start := time.Now()
	n, err := resp.Body.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("read: %v", err)
	}
	if n == 0 || !strings.Contains(string(buf[:n]), "data: 0") {
		t.Fatalf("expected first SSE chunk, got %q", buf[:n])
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Errorf("first chunk took %v; gateway is buffering", elapsed)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
}

func TestRejectsBadPath(t *testing.T) {
	gw, _, _ := newGateway(t, func(http.ResponseWriter, *http.Request) {})
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	cases := []string{
		"/v1/agents/",
		"/v1/agents/agents",
	}
	for _, p := range cases {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", p, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestHealthAndReady(t *testing.T) {
	gw, _, _ := newGateway(t, func(http.ResponseWriter, *http.Request) {})
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	for _, p := range []string{"/healthz", "/readyz"} {
		resp, err := http.Get(srv.URL + p)
		if err != nil || resp.StatusCode != 200 {
			t.Errorf("%s: err=%v code=%v", p, err, resp.StatusCode)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
}

func TestParseAgentPath(t *testing.T) {
	cases := []struct {
		path     string
		wantKey  types.NamespacedName
		wantRest string
		wantErr  bool
	}{
		{"/v1/agents/agents/travel", types.NamespacedName{Namespace: "agents", Name: "travel"}, "/", false},
		{"/v1/agents/agents/travel/", types.NamespacedName{Namespace: "agents", Name: "travel"}, "/", false},
		{"/v1/agents/agents/travel/a2a", types.NamespacedName{Namespace: "agents", Name: "travel"}, "/a2a", false},
		{"/v1/agents/agents/travel/a2a/messages", types.NamespacedName{Namespace: "agents", Name: "travel"}, "/a2a/messages", false},
		{"/v1/agents/agents/travel/.well-known/agent-card.json", types.NamespacedName{Namespace: "agents", Name: "travel"}, "/.well-known/agent-card.json", false},
		{"/v1/agents/", types.NamespacedName{}, "", true},
		{"/v1/agents/agents", types.NamespacedName{}, "", true},
		{"/other", types.NamespacedName{}, "", true},
	}
	for _, tc := range cases {
		key, rest, err := parseAgentPath(tc.path)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected error", tc.path)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tc.path, err)
			continue
		}
		if key != tc.wantKey || rest != tc.wantRest {
			t.Errorf("%s: got (%v, %q), want (%v, %q)", tc.path, key, rest, tc.wantKey, tc.wantRest)
		}
	}
}
