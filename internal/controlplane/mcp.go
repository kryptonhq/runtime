/*
Copyright 2026 Krypton Authors.
*/

package controlplane

import (
	"fmt"
	"io"
	"net/http"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/mcp"
)

// registerMCPRoutes mounts the tool-introspection endpoints. Only valid
// for agents with spec.protocol = "mcp"; other agents return 400.
//
//	GET  /v1/agents/{ns}/{name}/mcp/tools
//	POST /v1/agents/{ns}/{name}/mcp/tools/{tool}    body = JSON arguments
func (a *API) registerMCPRoutes(mux *http.ServeMux) {
	mux.Handle("GET /v1/agents/{namespace}/{name}/mcp/tools",
		observe("mcp_list_tools", http.HandlerFunc(a.mcpListTools)))
	mux.Handle("POST /v1/agents/{namespace}/{name}/mcp/tools/{tool}",
		observe("mcp_call_tool", http.HandlerFunc(a.mcpCallTool)))
}

func (a *API) mcpListTools(w http.ResponseWriter, r *http.Request) {
	agent, err := a.fetch(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	if !isMCP(agent) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("agent protocol is %q, not %q", agent.Spec.Protocol, kryptonv1alpha1.ProtocolMCP))
		return
	}

	client := mcp.NewClient(a.endpointFor(agent))
	if _, err := client.Initialize(r.Context()); err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("mcp initialize: %w", err))
		return
	}
	tools, err := client.ListTools(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("mcp tools/list: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

func (a *API) mcpCallTool(w http.ResponseWriter, r *http.Request) {
	agent, err := a.fetch(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	if !isMCP(agent) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("agent protocol is %q, not %q", agent.Spec.Protocol, kryptonv1alpha1.ProtocolMCP))
		return
	}

	args, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("read body: %w", err))
		return
	}
	if len(args) == 0 {
		args = []byte("{}")
	}

	client := mcp.NewClient(a.endpointFor(agent))
	if _, err := client.Initialize(r.Context()); err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("mcp initialize: %w", err))
		return
	}
	result, err := client.CallTool(r.Context(), r.PathValue("tool"), args)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func isMCP(agent *kryptonv1alpha1.Agent) bool {
	return agent.Spec.Protocol == kryptonv1alpha1.ProtocolMCP
}

// endpointFor resolves the MCP URL for an agent, honouring the test
// override on API when one is set.
func (a *API) endpointFor(agent *kryptonv1alpha1.Agent) string {
	if a.mcpEndpoint != nil {
		return a.mcpEndpoint(agent)
	}
	return agentMCPEndpoint(agent)
}

// agentMCPEndpoint returns the in-cluster URL where the MCP server's
// JSON-RPC endpoint lives. Falls back to spec.invocationPath, which
// defaults to "/" — sample servers should mount their JSON-RPC handler
// there or override.
func agentMCPEndpoint(agent *kryptonv1alpha1.Agent) string {
	path := agent.Spec.InvocationPath
	if path == "" {
		path = "/"
	}
	return fmt.Sprintf("http://%s.%s.svc:%d%s", agent.Name, agent.Namespace, agent.Spec.Port, path)
}
