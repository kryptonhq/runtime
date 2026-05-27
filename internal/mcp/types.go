/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package mcp is a minimal Model Context Protocol client for Krypton.
//
// We implement the streamable-HTTP transport described in the MCP
// specification (revision 2025-06-18): every request is a JSON-RPC 2.0
// POST to a single endpoint, the response is JSON. SSE responses are
// not used by the tool-introspection paths we need.
//
// Scope is intentionally narrow: initialize handshake, tools/list,
// tools/call. Anything else (resources, prompts, sampling, completions)
// is out of scope until a real consumer asks for it.
package mcp

import "encoding/json"

// JSONRPCVersion is the JSON-RPC version every MCP message uses.
const JSONRPCVersion = "2.0"

// ProtocolVersion is the MCP revision Krypton's client advertises.
const ProtocolVersion = "2025-06-18"

// Request is a JSON-RPC 2.0 request envelope.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification (no id, no response).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response envelope.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error mirrors the JSON-RPC error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Tool describes a single tool exposed by an MCP server.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ListToolsResult is the result of a tools/list call.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// CallToolParams is the params object of a tools/call request.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ContentItem is a single piece of content in a tools/call response.
// Only text content is fully modeled; image/resource pass through as
// raw fields.
type ContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"` // base64 for image/binary
}

// CallToolResult is the result of a tools/call.
type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// InitializeResult is what the server sends back from an initialize call.
// We don't enforce its shape beyond carrying it through.
type InitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities,omitempty"`
	ServerInfo      ServerInfo      `json:"serverInfo"`
}

// ServerInfo identifies the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}
