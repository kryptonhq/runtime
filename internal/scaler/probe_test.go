/*
Copyright 2026 Krypton Authors.
*/

package scaler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// startInflightServer launches a fake sidecar /_krypton/inflight server.
// It hands back a stable identifier ("pod-N") that the test plumbs into
// the fake Endpoints — the portRewriter then dispatches probe requests
// by that pseudo-IP to the right port.
func startInflightServer(t *testing.T, inflight int) (host string, port int, cleanup func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/_krypton/inflight") {
			t.Errorf("unexpected probe path: %q", r.URL.Path)
		}
		fmt.Fprintf(w, `{"inflight":%d}`, inflight)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	_, p, _ := net.SplitHostPort(u.Host)
	pn, _ := strconv.Atoi(p)
	return "", pn, func() {}
}

func probeScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return s
}

func TestHTTPLoadProbeSumsAcrossPods(t *testing.T) {
	_, port1, _ := startInflightServer(t, 3)
	_, port2, _ := startInflightServer(t, 5)

	// Each fake pod has a unique pseudo-IP. The rewriter resolves
	// pseudo-IP → real loopback port so the test exercises the per-pod
	// fan-out path without needing two listeners on the same TCP port.
	rewriter := &portRewriter{
		ports: map[string]int{"10.0.0.1": port1, "10.0.0.2": port2},
		base:  http.DefaultTransport,
	}

	cli := fake.NewClientBuilder().
		WithScheme(probeScheme(t)).
		WithObjects(&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "travel", Namespace: "agents"},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}, {IP: "10.0.0.2"}},
			}},
		}).
		Build()

	probe := &HTTPLoadProbe{
		Client:      cli,
		SidecarPort: 8888, // doesn't matter; rewriter overrides
		HTTPClient:  &http.Client{Transport: rewriter},
	}
	n, err := probe.AgentInflight(context.Background(), types.NamespacedName{Name: "travel", Namespace: "agents"})
	if err != nil {
		t.Fatalf("AgentInflight: %v", err)
	}
	if n != 8 {
		t.Errorf("sum = %d, want 8 (3+5)", n)
	}
}

func TestHTTPLoadProbeNoEndpointsReturnsZero(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(probeScheme(t)).Build()
	probe := &HTTPLoadProbe{Client: cli, SidecarPort: 8888, HTTPClient: http.DefaultClient}
	n, err := probe.AgentInflight(context.Background(), types.NamespacedName{Name: "travel", Namespace: "agents"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != 0 {
		t.Errorf("no endpoints → %d, want 0", n)
	}
}

func TestHTTPLoadProbeIgnoresPodErrors(t *testing.T) {
	_, port, _ := startInflightServer(t, 7)
	rewriter := &portRewriter{
		// The bad-pod address is intentionally absent from the map →
		// requests pass through unchanged, hitting nothing on
		// 10.0.0.99:8888 → connection error → counted as zero.
		ports: map[string]int{"10.0.0.1": port},
		base:  http.DefaultTransport,
	}
	cli := fake.NewClientBuilder().
		WithScheme(probeScheme(t)).
		WithObjects(&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "travel", Namespace: "agents"},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}, {IP: "10.0.0.99"}},
			}},
		}).
		Build()

	probe := &HTTPLoadProbe{
		Client:      cli,
		SidecarPort: 8888,
		HTTPClient: &http.Client{
			Transport: rewriter,
			Timeout:   200 * time.Millisecond,
		},
	}
	n, _ := probe.AgentInflight(context.Background(), types.NamespacedName{Name: "travel", Namespace: "agents"})
	if n != 7 {
		t.Errorf("got %d, want 7 (errored pod skipped)", n)
	}
}

// portRewriter swaps the destination port for outbound HTTP requests so we
// can pretend each httptest server lives on whatever port the probe
// requests — without actually binding the same port twice.
type portRewriter struct {
	ports map[string]int
	base  http.RoundTripper
}

func (p *portRewriter) RoundTrip(r *http.Request) (*http.Response, error) {
	host, _, err := net.SplitHostPort(r.URL.Host)
	if err != nil {
		return p.base.RoundTrip(r)
	}
	if newPort, ok := p.ports[host]; ok {
		r2 := r.Clone(r.Context())
		r2.URL.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(newPort))
		r2.Host = r2.URL.Host
		return p.base.RoundTrip(r2)
	}
	return p.base.RoundTrip(r)
}
