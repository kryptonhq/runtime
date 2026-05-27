/*
Copyright 2026 Krypton Authors.
*/

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeMCPServer is a tiny in-test MCP server speaking JSON-RPC over HTTP.
// It records the methods called for assertions and lets each test customize
// per-method responses.
type fakeMCPServer struct {
	t        *testing.T
	mu       sync.Mutex
	called   []string
	tools    []Tool
	callResp map[string]CallToolResult
}

func (f *fakeMCPServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.called = append(f.called, req.Method)
		f.mu.Unlock()

		// Notifications carry no id and need no response.
		if strings.HasPrefix(req.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "sess-1")
		resp := Response{JSONRPC: JSONRPCVersion, ID: req.ID}
		switch req.Method {
		case "initialize":
			resp.Result, _ = json.Marshal(InitializeResult{
				ProtocolVersion: ProtocolVersion,
				ServerInfo:      ServerInfo{Name: "fake-mcp", Version: "0.0.1"},
			})
		case "tools/list":
			resp.Result, _ = json.Marshal(ListToolsResult{Tools: f.tools})
		case "tools/call":
			var p CallToolParams
			_ = json.Unmarshal(req.Params, &p)
			r, ok := f.callResp[p.Name]
			if !ok {
				resp.Error = &Error{Code: -32601, Message: "unknown tool"}
				break
			}
			resp.Result, _ = json.Marshal(r)
		default:
			resp.Error = &Error{Code: -32601, Message: "method not found"}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func newFake(t *testing.T) (*Client, *fakeMCPServer) {
	t.Helper()
	f := &fakeMCPServer{
		t:        t,
		callResp: map[string]CallToolResult{},
	}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return NewClient(srv.URL), f
}

func TestInitialize(t *testing.T) {
	c, f := newFake(t)
	res, err := c.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if res.ServerInfo.Name != "fake-mcp" {
		t.Errorf("server name = %q, want fake-mcp", res.ServerInfo.Name)
	}
	if c.sessionID != "sess-1" {
		t.Errorf("session id not captured: %q", c.sessionID)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.called) != 2 || f.called[0] != "initialize" || f.called[1] != "notifications/initialized" {
		t.Errorf("methods called = %v, want [initialize, notifications/initialized]", f.called)
	}
}

func TestListTools(t *testing.T) {
	c, f := newFake(t)
	f.tools = []Tool{
		{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "add", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	_, _ = c.Initialize(context.Background())
	got, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(got) != 2 || got[0].Name != "echo" || got[1].Name != "add" {
		t.Errorf("tools = %+v", got)
	}
}

func TestCallTool(t *testing.T) {
	c, f := newFake(t)
	f.callResp["echo"] = CallToolResult{
		Content: []ContentItem{{Type: "text", Text: "hello"}},
	}
	_, _ = c.Initialize(context.Background())
	res, err := c.CallTool(context.Background(), "echo", json.RawMessage(`{"message":"hi"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "hello" {
		t.Errorf("content = %+v", res.Content)
	}
}

func TestCallToolUnknownReturnsError(t *testing.T) {
	c, _ := newFake(t)
	_, _ = c.Initialize(context.Background())
	_, err := c.CallTool(context.Background(), "missing", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("err = %v, want unknown tool", err)
	}
}

func TestSSEResponseTolerated(t *testing.T) {
	// Some MCP servers wrap responses in SSE frames even for short calls.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[{\"name\":\"x\",\"inputSchema\":{}}]}}\n\n"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools (SSE-framed): %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "x" {
		t.Errorf("tools = %+v", tools)
	}
}

func TestHTTPErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	_, err := c.ListTools(context.Background())
	if err == nil {
		t.Fatal("expected error from 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want HTTP 500", err)
	}
}
