/*
Copyright 2026 Krypton Authors.
*/

package scaler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadProbe reports the total in-flight request count across an agent's
// pods. Production: HTTPLoadProbe queries each sidecar directly. Tests
// supply a fake.
type LoadProbe interface {
	AgentInflight(ctx context.Context, key types.NamespacedName) (int, error)
}

// HTTPLoadProbe queries the krypton-proxy sidecar on each ready pod for
// the agent's Endpoints object.
type HTTPLoadProbe struct {
	Client     client.Client
	HTTPClient *http.Client
	// SidecarPort matches the sidecar's listen port (8888 by default; see
	// controller's sidecarPort constant).
	SidecarPort int
}

// NewHTTPLoadProbe returns a probe with a sensible HTTP client.
func NewHTTPLoadProbe(c client.Client, sidecarPort int) *HTTPLoadProbe {
	return &HTTPLoadProbe{
		Client:      c,
		SidecarPort: sidecarPort,
		HTTPClient:  &http.Client{Timeout: 2 * time.Second},
	}
}

// AgentInflight fans out across the agent's ready pod IPs and sums each
// sidecar's reported in-flight count. Errors on individual pods are
// silently dropped — undercounting is preferable to refusing to make a
// decision.
func (p *HTTPLoadProbe) AgentInflight(ctx context.Context, key types.NamespacedName) (int, error) {
	var eps corev1.Endpoints
	if err := p.Client.Get(ctx, key, &eps); err != nil {
		// No endpoints object yet — treat as zero pods, zero load.
		return 0, nil
	}
	total := 0
	for _, subset := range eps.Subsets {
		for _, addr := range subset.Addresses {
			n, err := p.probe(ctx, addr.IP)
			if err != nil {
				continue
			}
			total += n
		}
	}
	return total, nil
}

func (p *HTTPLoadProbe) probe(ctx context.Context, ip string) (int, error) {
	port := p.SidecarPort
	if port == 0 {
		port = 8888
	}
	url := fmt.Sprintf("http://%s:%d/_krypton/inflight", ip, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Inflight int `json:"inflight"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}
	return body.Inflight, nil
}
