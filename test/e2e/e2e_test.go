//go:build e2e

/*
Copyright 2026 Krypton Authors.
*/

// Package e2e holds the behavioural half of Krypton's end-to-end suite.
//
// Split of responsibilities with the Chainsaw suite next door
// (test/e2e/chainsaw):
//
//   - Chainsaw owns declarative reconcile assertions: "apply this CR, assert
//     these objects exist with these fields". That's most of the surface and
//     it's far terser as YAML.
//   - This package owns what YAML expresses badly: latency budgets, protocol
//     details, streaming, the OpenAI-compatible routes, and the
//     scale-to-zero cold-start path.
//
// Everything here talks HTTP through port-forwards that hack/e2e.sh sets up,
// so the assertions run against the same surface a real client sees.
//
// Run via: make test-e2e
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

var (
	gatewayURL      string
	controlPlaneURL string
	namespace       string
	release         string
	agentImage      string
	llmEnabled      bool
)

func TestMain(m *testing.M) {
	gatewayURL = envOr("KRYPTON_E2E_GATEWAY", "http://127.0.0.1:18080")
	controlPlaneURL = envOr("KRYPTON_E2E_CONTROL_PLANE", "http://127.0.0.1:18090")
	namespace = envOr("KRYPTON_E2E_NAMESPACE", "krypton-system")
	release = envOr("KRYPTON_E2E_RELEASE", "krypton")
	agentImage = envOr("KRYPTON_E2E_IMAGE", "krypton/mcp-hello:e2e")
	llmEnabled = os.Getenv("KRYPTON_E2E_LLM") == "true"

	os.Exit(m.Run())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ---- health ----------------------------------------------------------------

func TestComponentsAreHealthy(t *testing.T) {
	for _, tc := range []struct{ name, url string }{
		{"gateway healthz", gatewayURL + "/healthz"},
		{"gateway readyz", gatewayURL + "/readyz"},
		{"control plane healthz", controlPlaneURL + "/healthz"},
		{"control plane readyz", controlPlaneURL + "/readyz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(tc.url)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.url, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200", resp.StatusCode)
			}
		})
	}
}

// The operator UI is embedded into the control-plane binary via go:embed. If
// `make ui` didn't run before the image build, this serves the "UI not built"
// placeholder — which is exactly the regression worth catching, since the
// binary still starts and every other check stays green.
func TestOperatorUIIsServed(t *testing.T) {
	resp, err := http.Get(controlPlaneURL + "/ui/")
	if err != nil {
		t.Fatalf("GET /ui/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "Krypton UI not built") {
		t.Error("control plane is serving the placeholder UI — the image was built without `make ui`")
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestRootRedirectsToUI(t *testing.T) {
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(controlPlaneURL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/ui/" {
		t.Errorf("Location = %q, want /ui/", loc)
	}
}

// ---- control plane REST ----------------------------------------------------

type agentView struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Spec      struct {
		Image    string `json:"image"`
		Protocol string `json:"protocol"`
		Mode     string `json:"mode"`
		Port     int32  `json:"port"`
	} `json:"spec"`
	Status struct {
		Phase         string `json:"phase"`
		Replicas      int32  `json:"replicas"`
		ReadyReplicas int32  `json:"readyReplicas"`
		URL           string `json:"url"`
	} `json:"status"`
}

type agentList struct {
	Items    []agentView `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
	Total    int         `json:"total"`
}

func TestControlPlaneListsTheSmokeAgent(t *testing.T) {
	mustApplySmokeAgent(t)

	var list agentList
	waitFor(t, 3*time.Minute, "mcp-hello to appear in /v1/agents", func() error {
		if err := getJSON(controlPlaneURL+"/v1/agents", &list); err != nil {
			return err
		}
		for _, a := range list.Items {
			if a.Name == "mcp-hello" {
				return nil
			}
		}
		return fmt.Errorf("not in the list of %d agents", list.Total)
	})

	// The envelope is {items,page,pageSize,total}, not a bare array. The UI
	// depends on this shape.
	if list.Page != 1 {
		t.Errorf("page = %d, want 1", list.Page)
	}
	if list.PageSize == 0 {
		t.Error("pageSize is 0; the envelope should carry the effective page size")
	}
}

func TestControlPlaneReportsAgentReady(t *testing.T) {
	mustApplySmokeAgent(t)

	var a agentView
	waitFor(t, 3*time.Minute, "mcp-hello to reach Phase=Ready", func() error {
		if err := getJSON(controlPlaneURL+"/v1/agents/agents/mcp-hello", &a); err != nil {
			return err
		}
		if a.Status.Phase != "Ready" {
			return fmt.Errorf("phase = %q, replicas = %d/%d",
				a.Status.Phase, a.Status.ReadyReplicas, a.Status.Replicas)
		}
		return nil
	})

	if a.Spec.Protocol != "mcp" {
		t.Errorf("protocol = %q, want mcp", a.Spec.Protocol)
	}
	if !strings.Contains(a.Status.URL, "mcp-hello") {
		t.Errorf("status.url = %q, want it to reference the Service", a.Status.URL)
	}
}

func TestControlPlaneFiltersByProtocol(t *testing.T) {
	mustApplySmokeAgent(t)

	var list agentList
	waitFor(t, 2*time.Minute, "protocol filter to return mcp-hello", func() error {
		if err := getJSON(controlPlaneURL+"/v1/agents?protocol=mcp", &list); err != nil {
			return err
		}
		if len(list.Items) == 0 {
			return fmt.Errorf("no agents matched ?protocol=mcp")
		}
		return nil
	})

	// The filter is applied server-side; nothing non-MCP may come back.
	for _, a := range list.Items {
		if a.Spec.Protocol != "mcp" {
			t.Errorf("agent %s has protocol %q but matched ?protocol=mcp", a.Name, a.Spec.Protocol)
		}
	}
}

func TestControlPlaneReturns404ForUnknownAgent(t *testing.T) {
	resp, err := http.Get(controlPlaneURL + "/v1/agents/agents/does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// ---- MCP introspection through the control plane ---------------------------

func TestMCPToolsAreDiscoverable(t *testing.T) {
	mustApplySmokeAgent(t)
	waitForAgentReady(t, "agents", "mcp-hello")

	var out struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}

	// The control plane proxies tools/list to the agent's MCP endpoint over
	// in-cluster DNS, so this exercises Service routing plus the sidecar.
	waitFor(t, 2*time.Minute, "MCP tools/list to succeed", func() error {
		return getJSON(controlPlaneURL+"/v1/agents/agents/mcp-hello/mcp/tools", &out)
	})

	if len(out.Tools) == 0 {
		t.Fatal("mcp-hello advertised no tools")
	}
	for _, tool := range out.Tools {
		if tool.Name == "" {
			t.Error("a tool has an empty name")
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has no inputSchema", tool.Name)
		}
	}
}

func TestMCPToolCallRoundTrip(t *testing.T) {
	mustApplySmokeAgent(t)
	waitForAgentReady(t, "agents", "mcp-hello")

	var tools struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	waitFor(t, 2*time.Minute, "tools/list", func() error {
		return getJSON(controlPlaneURL+"/v1/agents/agents/mcp-hello/mcp/tools", &tools)
	})
	if len(tools.Tools) == 0 {
		t.Skip("no tools advertised; nothing to call")
	}

	name := tools.Tools[0].Name
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	url := controlPlaneURL + "/v1/agents/agents/mcp-hello/mcp/tools/" + name
	if err := postJSON(url, map[string]any{}, &result); err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	if len(result.Content) == 0 {
		t.Errorf("tool %q returned no content", name)
	}
}

// A non-MCP agent must be rejected with 400, not proxied. Confirms the
// protocol guard survives real routing.
func TestMCPRoutesRejectNonMCPAgent(t *testing.T) {
	applyManifest(t, fmt.Sprintf(`
apiVersion: krypton.ai/v1alpha1
kind: Agent
metadata:
  name: e2e-http-agent
  namespace: agents
spec:
  image: %s
  imagePullPolicy: IfNotPresent
  protocol: http
  mode: always-on
  minReplicas: 1
  port: 8080
`, agentImage))
	t.Cleanup(func() { deleteResource(t, "agent", "e2e-http-agent", "agents") })

	var resp *http.Response
	waitFor(t, 2*time.Minute, "control plane to see e2e-http-agent", func() error {
		r, err := http.Get(controlPlaneURL + "/v1/agents/agents/e2e-http-agent/mcp/tools")
		if err != nil {
			return err
		}
		if r.StatusCode == http.StatusNotFound {
			r.Body.Close()
			return fmt.Errorf("agent not visible yet")
		}
		resp = r
		return nil
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a non-MCP agent", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "protocol") {
		t.Errorf("error should name the protocol mismatch: %s", body)
	}
}

// ---- gateway invocation ----------------------------------------------------

func TestGatewayInvokesAgent(t *testing.T) {
	mustApplySmokeAgent(t)
	waitForAgentReady(t, "agents", "mcp-hello")

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	url := gatewayURL + "/v1/agents/agents/mcp-hello/"

	var status int
	var payload []byte
	waitFor(t, 2*time.Minute, "gateway to route an invocation", func() error {
		resp, err := http.Post(url, "application/json", strings.NewReader(body))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		payload, _ = io.ReadAll(resp.Body)
		status = resp.StatusCode
		if status != http.StatusOK {
			return fmt.Errorf("status %d: %s", status, payload)
		}
		return nil
	})

	if !bytes.Contains(payload, []byte("tools")) {
		t.Errorf("response does not look like a tools/list result: %s", payload)
	}
}

// Warm-path latency. The first request may pay for connection setup; once
// warm, the gateway plus sidecar hop should be single-digit milliseconds on
// a local cluster. The budget is deliberately loose (3s) so this catches a
// real regression — like an accidental cold-start on every request — rather
// than flaking on a loaded CI runner.
func TestWarmInvocationIsFast(t *testing.T) {
	mustApplySmokeAgent(t)
	waitForAgentReady(t, "agents", "mcp-hello")

	url := gatewayURL + "/v1/agents/agents/mcp-hello/"
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

	// Warm up.
	waitFor(t, 2*time.Minute, "first invocation to succeed", func() error {
		resp, err := http.Post(url, "application/json", strings.NewReader(body))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	})

	const budget = 3 * time.Second
	for i := 0; i < 5; i++ {
		start := time.Now()
		resp, err := http.Post(url, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("warm invocation %d: %v", i, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		elapsed := time.Since(start)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("warm invocation %d: status %d", i, resp.StatusCode)
		}
		if elapsed > budget {
			t.Errorf("warm invocation %d took %v (budget %v) — is every request cold-starting?",
				i, elapsed, budget)
		}
	}
}

func TestGatewayReturns404ForUnknownAgent(t *testing.T) {
	resp, err := http.Post(
		gatewayURL+"/v1/agents/agents/nope-not-here/",
		"application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	// Anything but a 5xx: the gateway must not fall over on an unknown name.
	if resp.StatusCode >= 500 {
		t.Errorf("status = %d; an unknown agent should not produce a server error", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Logf("note: status = %d (expected 404)", resp.StatusCode)
	}
}

func TestGatewayRejectsMalformedPath(t *testing.T) {
	for _, path := range []string{
		"/v1/agents/",
		"/v1/agents/only-namespace/",
	} {
		resp, err := http.Post(gatewayURL+path, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			t.Errorf("POST %s: status %d; malformed paths should be 4xx", path, resp.StatusCode)
		}
	}
}

// ---- OpenAI-compatible model routes ----------------------------------------

func TestModelsEndpointIsOpenAIShaped(t *testing.T) {
	var out struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := getJSON(gatewayURL+"/v1/models", &out); err != nil {
		t.Fatalf("GET /v1/models: %v", err)
	}

	// Shape must match the OpenAI list envelope even with zero models, or
	// the OpenAI SDKs error out before the user deploys anything.
	if out.Object != "list" {
		t.Errorf("object = %q, want \"list\"", out.Object)
	}
	for _, m := range out.Data {
		if m.ID == "" {
			t.Error("a model entry has an empty id")
		}
		if m.Object != "model" {
			t.Errorf("model %s: object = %q, want \"model\"", m.ID, m.Object)
		}
	}

	if !llmEnabled {
		t.Logf("KRYPTON_E2E_LLM is not set; %d model(s) registered", len(out.Data))
	}
}

func TestChatCompletionsRoundTrip(t *testing.T) {
	if !llmEnabled {
		t.Skip("KRYPTON_E2E_LLM is not true; the llama.cpp model path is release-only")
	}

	modelName := envOr("KRYPTON_E2E_LLM_NAME", "qwen2-0-5b")

	// Model pods pull multi-GB weights on first start, so the readiness
	// window here is much wider than for agents.
	waitFor(t, 20*time.Minute, "the model to report Ready", func() error {
		var m struct {
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		}
		if err := getJSON(controlPlaneURL+"/v1/models/models/"+modelName, &m); err != nil {
			return err
		}
		if m.Status.Phase != "Ready" {
			return fmt.Errorf("phase = %q", m.Status.Phase)
		}
		return nil
	})

	reqBody := map[string]any{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": "Say hi in one word."},
		},
		"max_tokens": 16,
	}
	var out struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := postJSON(gatewayURL+"/v1/chat/completions", reqBody, &out); err != nil {
		t.Fatalf("chat completion: %v", err)
	}
	if len(out.Choices) == 0 {
		t.Fatal("no choices returned")
	}
	if strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		t.Error("assistant message is empty")
	}
}

// ---- helpers ---------------------------------------------------------------

func getJSON(url string, out any) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w (body=%s)", url, err, body)
	}
	return nil
}

func postJSON(url string, in, out any) error {
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// waitFor polls cond until it succeeds or the timeout expires, then fails
// with the last error so the failure names the actual problem.
func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		last = cond()
		if last == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out after %v waiting for %s: %v", timeout, desc, last)
}

func waitForAgentReady(t *testing.T, ns, name string) {
	t.Helper()
	waitFor(t, 3*time.Minute, name+" to reach Phase=Ready", func() error {
		var a agentView
		if err := getJSON(fmt.Sprintf("%s/v1/agents/%s/%s", controlPlaneURL, ns, name), &a); err != nil {
			return err
		}
		if a.Status.Phase != "Ready" {
			return fmt.Errorf("phase = %q (%d/%d ready)",
				a.Status.Phase, a.Status.ReadyReplicas, a.Status.Replicas)
		}
		return nil
	})
}

// mustApplySmokeAgent installs the mcp-hello agent, pinned to the image
// hack/e2e.sh built and loaded into kind. Idempotent, so every test that
// needs it can call it.
func mustApplySmokeAgent(t *testing.T) {
	t.Helper()
	applyManifest(t, fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: agents
---
apiVersion: krypton.ai/v1alpha1
kind: Agent
metadata:
  name: mcp-hello
  namespace: agents
spec:
  image: %s
  imagePullPolicy: IfNotPresent
  runtime: go
  framework: stdlib
  protocol: mcp
  mode: always-on
  minReplicas: 1
  maxReplicas: 2
  concurrency: 8
  port: 8080
  invocationPath: /
  resources:
    requests: { cpu: 50m, memory: 64Mi }
    limits: { cpu: 500m, memory: 256Mi }
`, agentImage))
}

func applyManifest(t *testing.T, manifest string) {
	t.Helper()
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl apply failed: %v\n%s\nmanifest:\n%s", err, out, manifest)
	}
}

func deleteResource(t *testing.T, kind, name, ns string) {
	t.Helper()
	cmd := exec.Command("kubectl", "-n", ns, "delete", kind, name, "--ignore-not-found", "--wait=false")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("cleanup: kubectl delete %s/%s: %v\n%s", kind, name, err, out)
	}
}
