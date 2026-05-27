/*
Copyright 2026 Krypton Authors.
*/

package sidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newTestProxy(t *testing.T, concurrency int, upstream http.HandlerFunc) (*Proxy, *httptest.Server) {
	t.Helper()
	upstreamSrv := httptest.NewServer(upstream)
	t.Cleanup(upstreamSrv.Close)

	cfg := Config{
		AgentName:       "test",
		AgentNamespace:  "default",
		ListenAddr:      ":0",
		UpstreamURL:     upstreamSrv.URL,
		Concurrency:     concurrency,
		Mode:            ModeServerless,
		IdleTimeout:     50 * time.Millisecond,
		ShutdownTimeout: time.Second,
	}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	return p, upstreamSrv
}

func TestProxyForwardsToUpstream(t *testing.T) {
	called := false
	p, _ := newTestProxy(t, 4, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/invoke" {
			t.Errorf("upstream path = %q, want /invoke", r.URL.Path)
		}
		fmt.Fprint(w, "pong")
	})
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/invoke")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" || resp.StatusCode != 200 {
		t.Fatalf("got %d %q", resp.StatusCode, string(body))
	}
	if !called {
		t.Fatal("upstream not called")
	}
}

func TestHealthAndReady(t *testing.T) {
	p, _ := newTestProxy(t, 4, func(w http.ResponseWriter, _ *http.Request) {})
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("/healthz: err=%v code=%v", err, resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = http.Get(srv.URL + "/readyz")
	if resp.StatusCode != 200 {
		t.Fatalf("/readyz before shutdown = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	p.MarkShutdown()
	resp, _ = http.Get(srv.URL + "/readyz")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/readyz after shutdown = %d, want 503", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestConcurrencyLimit(t *testing.T) {
	release := make(chan struct{})
	p, _ := newTestProxy(t, 2, func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	// Fire 2 long-running requests to fill the semaphore.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/x")
			if err == nil {
				resp.Body.Close()
			}
		}()
	}
	// Wait until both are accounted for in-flight.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && p.Inflight() < 2 {
		time.Sleep(2 * time.Millisecond)
	}
	if p.Inflight() != 2 {
		t.Fatalf("inflight = %d, want 2", p.Inflight())
	}

	// Third request must be rejected immediately with 503.
	resp, err := http.Get(srv.URL + "/x")
	if err != nil {
		t.Fatalf("third GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Error("missing Retry-After header on 503")
	}

	close(release)
	wg.Wait()
}

func TestInflightEndpoint(t *testing.T) {
	p, _ := newTestProxy(t, 4, func(w http.ResponseWriter, _ *http.Request) {})
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/_krypton/inflight")
	if err != nil {
		t.Fatalf("get inflight: %v", err)
	}
	defer resp.Body.Close()
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if int(got["concurrency"].(float64)) != 4 {
		t.Errorf("concurrency = %v, want 4", got["concurrency"])
	}
}

func TestIsIdle(t *testing.T) {
	p, _ := newTestProxy(t, 4, func(w http.ResponseWriter, _ *http.Request) {})
	// Just constructed: lastActivity is now, so not idle.
	if p.IsIdle(time.Now()) {
		t.Fatal("freshly constructed proxy reported idle")
	}
	// Force time past idle timeout.
	if !p.IsIdle(time.Now().Add(p.cfg.IdleTimeout + 10*time.Millisecond)) {
		t.Fatal("proxy with no activity past timeout should be idle")
	}
	// Always-on mode never idles.
	p.cfg.Mode = ModeAlwaysOn
	if p.IsIdle(time.Now().Add(time.Hour)) {
		t.Fatal("always-on must never report idle")
	}
}

func TestDrainAndShutdown(t *testing.T) {
	gate := make(chan struct{})
	done := make(chan struct{})
	p, _ := newTestProxy(t, 4, func(w http.ResponseWriter, _ *http.Request) {
		<-gate
	})
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	go func() {
		resp, err := http.Get(srv.URL + "/x")
		if err == nil {
			resp.Body.Close()
		}
		close(done)
	}()
	// Wait for in-flight to land.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && p.Inflight() == 0 {
		time.Sleep(2 * time.Millisecond)
	}
	if p.Inflight() != 1 {
		t.Fatalf("inflight = %d, want 1", p.Inflight())
	}

	p.MarkShutdown()
	// Drain with a tight timeout that should fire while the request is still in-flight.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := p.DrainAndShutdown(ctx); err == nil {
		t.Fatal("DrainAndShutdown should have hit ctx deadline while request is in-flight")
	}

	// Release the upstream, then drain should complete cleanly.
	close(gate)
	<-done
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := p.DrainAndShutdown(ctx2); err != nil {
		t.Fatalf("clean drain returned: %v", err)
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("KRYPTON_AGENT_NAME", "travel")
	t.Setenv("KRYPTON_AGENT_NAMESPACE", "agents")
	t.Setenv("KRYPTON_CONCURRENCY", "32")
	t.Setenv("KRYPTON_MODE", "always-on")
	t.Setenv("KRYPTON_IDLE_TIMEOUT", "120s")
	t.Setenv("KRYPTON_UPSTREAM_URL", "http://127.0.0.1:9000")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.AgentName != "travel" || cfg.Concurrency != 32 || cfg.Mode != ModeAlwaysOn {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.IdleTimeout != 2*time.Minute {
		t.Errorf("idle = %v, want 2m", cfg.IdleTimeout)
	}
}

func TestConfigFromEnvInvalid(t *testing.T) {
	t.Setenv("KRYPTON_CONCURRENCY", "abc")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected error on non-numeric concurrency")
	}
	t.Setenv("KRYPTON_CONCURRENCY", "8")
	t.Setenv("KRYPTON_MODE", "bogus")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected error on unknown mode")
	}
}
