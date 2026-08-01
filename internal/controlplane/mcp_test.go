/*
Copyright 2026 Krypton Authors.
*/

package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

// fakeMCPServer stands in for an agent's MCP endpoint. It speaks just
// enough JSON-RPC for initialize / tools/list / tools/call. Behaviour per
// method is injectable so tests can drive the error branches.
type fakeMCPServer struct {
	// responses maps a JSON-RPC method to the raw `result` payload to
	// return. A method absent from the map gets a JSON-RPC error.
	responses map[string]string
	// httpStatus, when non-zero, is returned for every request instead of
	// a JSON-RPC envelope.
	httpStatus int
	// sse wraps the response in an SSE `data:` frame.
	sse bool

	calls []string
}

func (f *fakeMCPServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.calls = append(f.calls, req.Method)

		// Notifications carry no id and expect no body.
		if strings.HasPrefix(req.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		if f.httpStatus != 0 {
			w.WriteHeader(f.httpStatus)
			_, _ = w.Write([]byte("upstream exploded"))
			return
		}

		result, ok := f.responses[req.Method]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"error":{"code":-32601,"message":"method not found: %s"}}`, req.ID, req.Method)
			return
		}

		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, result)
		if f.sse {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
}

const initResult = `{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"fake","version":"0"}}`

// mcpAPI wires an API whose MCP endpoint points at srv instead of at
// in-cluster Service DNS.
func mcpAPI(t *testing.T, srv *httptest.Server, agents ...*kryptonv1alpha1.Agent) *API {
	t.Helper()
	objs := make([]client.Object, 0, len(agents))
	for _, a := range agents {
		objs = append(objs, a)
	}
	return &API{
		Client:      testClient(t, objs...),
		mcpEndpoint: func(*kryptonv1alpha1.Agent) string { return srv.URL },
	}
}

func newMCPAgent(name, ns string) *kryptonv1alpha1.Agent {
	a := newSampleAgent(name, ns)
	a.Spec.Protocol = kryptonv1alpha1.ProtocolMCP
	return a
}

func TestMCPListTools(t *testing.T) {
	fake := &fakeMCPServer{responses: map[string]string{
		"initialize": initResult,
		"tools/list": `{"tools":[{"name":"echo","description":"echo back","inputSchema":{"type":"object"}}]}`,
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	api := mcpAPI(t, srv, newMCPAgent("mcp-hello", "agents"))
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/agents/agents/mcp-hello/mcp/tools", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "echo" {
		t.Fatalf("tools = %+v, want one tool named echo", got.Tools)
	}
	// initialize must precede tools/list, otherwise spec-compliant servers
	// reject the call.
	if len(fake.calls) < 2 || fake.calls[0] != "initialize" {
		t.Errorf("call order = %v, want initialize first", fake.calls)
	}
}

// The MCP client tolerates servers that wrap the JSON-RPC envelope in an
// SSE frame; the control plane must too.
func TestMCPListToolsOverSSE(t *testing.T) {
	fake := &fakeMCPServer{
		sse: true,
		responses: map[string]string{
			"initialize": initResult,
			"tools/list": `{"tools":[{"name":"streamed","inputSchema":{}}]}`,
		},
	}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	api := mcpAPI(t, srv, newMCPAgent("mcp-hello", "agents"))
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/agents/agents/mcp-hello/mcp/tools", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "streamed") {
		t.Errorf("body does not contain the tool name: %s", rec.Body.String())
	}
}

func TestMCPListToolsRejectsNonMCPProtocol(t *testing.T) {
	srv := httptest.NewServer((&fakeMCPServer{}).handler())
	defer srv.Close()

	// Default sample agent speaks a2a, not mcp.
	a := newSampleAgent("travel", "agents")
	a.Spec.Protocol = kryptonv1alpha1.ProtocolA2A

	api := mcpAPI(t, srv, a)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/agents/agents/travel/mcp/tools", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	// The message names both the actual and the required protocol so the
	// operator knows what to change. JSON-escaped in the response body.
	if want := `agent protocol is \"a2a\", not \"mcp\"`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body = %s\nwant it to contain: %s", rec.Body.String(), want)
	}
}

func TestMCPListToolsMissingAgent(t *testing.T) {
	srv := httptest.NewServer((&fakeMCPServer{}).handler())
	defer srv.Close()

	api := mcpAPI(t, srv)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/agents/agents/ghost/mcp/tools", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

// A reachable-but-broken MCP server is an upstream failure, not our bug:
// it must surface as 502, never 500.
func TestMCPListToolsUpstreamFailureIs502(t *testing.T) {
	tests := []struct {
		name string
		fake *fakeMCPServer
	}{
		{
			name: "initialize returns HTTP 500",
			fake: &fakeMCPServer{httpStatus: http.StatusInternalServerError},
		},
		{
			name: "initialize ok but tools/list unsupported",
			fake: &fakeMCPServer{responses: map[string]string{"initialize": initResult}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.fake.handler())
			defer srv.Close()

			api := mcpAPI(t, srv, newMCPAgent("mcp-hello", "agents"))
			rec := httptest.NewRecorder()
			api.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/agents/agents/mcp-hello/mcp/tools", nil))

			if rec.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502 (body=%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestMCPCallTool(t *testing.T) {
	fake := &fakeMCPServer{responses: map[string]string{
		"initialize": initResult,
		"tools/call": `{"content":[{"type":"text","text":"pong"}],"isError":false}`,
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	api := mcpAPI(t, srv, newMCPAgent("mcp-hello", "agents"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/v1/agents/agents/mcp-hello/mcp/tools/echo",
		strings.NewReader(`{"message":"ping"}`))
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "pong") {
		t.Errorf("body = %s, want it to contain pong", rec.Body.String())
	}
}

// An empty body is legal — tools with no required arguments are called
// with no payload, and the handler substitutes "{}".
func TestMCPCallToolEmptyBodyBecomesEmptyObject(t *testing.T) {
	fake := &fakeMCPServer{responses: map[string]string{
		"initialize": initResult,
		"tools/call": `{"content":[{"type":"text","text":"ok"}]}`,
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	api := mcpAPI(t, srv, newMCPAgent("mcp-hello", "agents"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/v1/agents/agents/mcp-hello/mcp/tools/now", strings.NewReader(""))
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestMCPCallToolRejectsNonMCPProtocol(t *testing.T) {
	srv := httptest.NewServer((&fakeMCPServer{}).handler())
	defer srv.Close()

	a := newSampleAgent("travel", "agents")
	a.Spec.Protocol = kryptonv1alpha1.ProtocolHTTP

	api := mcpAPI(t, srv, a)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/v1/agents/agents/travel/mcp/tools/echo", strings.NewReader("{}"))
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestMCPCallToolMissingAgent(t *testing.T) {
	srv := httptest.NewServer((&fakeMCPServer{}).handler())
	defer srv.Close()

	api := mcpAPI(t, srv)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/v1/agents/agents/ghost/mcp/tools/echo", strings.NewReader("{}"))
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// A tool that reports failure via JSON-RPC error (rather than
// isError:true) is an upstream problem → 502.
func TestMCPCallToolUpstreamErrorIs502(t *testing.T) {
	fake := &fakeMCPServer{responses: map[string]string{"initialize": initResult}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	api := mcpAPI(t, srv, newMCPAgent("mcp-hello", "agents"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/v1/agents/agents/mcp-hello/mcp/tools/nope", strings.NewReader("{}"))
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestIsMCP(t *testing.T) {
	tests := []struct {
		protocol kryptonv1alpha1.Protocol
		want     bool
	}{
		{kryptonv1alpha1.ProtocolMCP, true},
		{kryptonv1alpha1.ProtocolA2A, false},
		{kryptonv1alpha1.ProtocolHTTP, false},
		{"", false},
	}
	for _, tc := range tests {
		a := &kryptonv1alpha1.Agent{}
		a.Spec.Protocol = tc.protocol
		if got := isMCP(a); got != tc.want {
			t.Errorf("isMCP(%q) = %v, want %v", tc.protocol, got, tc.want)
		}
	}
}

func TestAgentMCPEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		invocationPath string
		port           int32
		want           string
	}{
		{
			name:           "explicit path",
			invocationPath: "/mcp",
			port:           8080,
			want:           "http://mcp-hello.agents.svc:8080/mcp",
		},
		{
			// spec.invocationPath is optional; empty must not produce a
			// URL with no path ("...svc:8080"), which some servers 404.
			name:           "empty path defaults to root",
			invocationPath: "",
			port:           9000,
			want:           "http://mcp-hello.agents.svc:9000/",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newMCPAgent("mcp-hello", "agents")
			a.Spec.InvocationPath = tc.invocationPath
			a.Spec.Port = tc.port
			if got := agentMCPEndpoint(a); got != tc.want {
				t.Errorf("agentMCPEndpoint() = %q, want %q", got, tc.want)
			}
		})
	}
}

// endpointFor falls back to in-cluster DNS when no override is installed,
// which is the production path.
func TestEndpointForFallsBackToClusterDNS(t *testing.T) {
	api := &API{}
	a := newMCPAgent("mcp-hello", "agents")
	a.Spec.Port = 8080
	a.Spec.InvocationPath = "/"
	if got, want := api.endpointFor(a), "http://mcp-hello.agents.svc:8080/"; got != want {
		t.Errorf("endpointFor() = %q, want %q", got, want)
	}
}
