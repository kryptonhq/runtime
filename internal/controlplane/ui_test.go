/*
Copyright 2026 Krypton Authors.
*/

package controlplane

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIServesIndex(t *testing.T) {
	srv := httptest.NewServer(UI())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ui/")
	if err != nil {
		t.Fatalf("GET /ui/: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	// Built UI contains <div id="root"></div>; placeholder contains
	// "Krypton UI not built". Either is acceptable in CI before make ui
	// runs, but for this test we require the real artifact.
	if strings.Contains(string(body), "UI not built") {
		t.Fatal("UI not built — `make ui` first")
	}
	if !strings.Contains(string(body), `id="root"`) {
		t.Errorf("response doesn't look like the built UI: %q", string(body)[:min(200, len(body))])
	}
}

func TestUIFallsBackToIndexForSPARoutes(t *testing.T) {
	srv := httptest.NewServer(UI())
	defer srv.Close()

	// /ui/agents/default/foo is a client-side route; the server must
	// return index.html, not 404.
	resp, err := http.Get(srv.URL + "/ui/agents/default/foo")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200 (SPA fallback)", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `id="root"`) {
		t.Errorf("SPA fallback didn't return index.html: %q", string(body)[:min(200, len(body))])
	}
}

func TestUIServesAssets(t *testing.T) {
	srv := httptest.NewServer(UI())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ui/favicon.svg")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("favicon status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "svg") {
		t.Errorf("favicon content-type = %q", got)
	}
}
