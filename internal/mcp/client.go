/*
Copyright 2026 Krypton Authors.
*/

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Client is a one-shot MCP client over streamable HTTP. It is not safe
// for concurrent use across separate sessions because the JSON-RPC id
// counter is shared — construct one client per logical session
// (initialize → list/call → done).
type Client struct {
	Endpoint string
	HTTP     *http.Client

	id atomic.Int64

	// sessionID is set from Mcp-Session-Id on the initialize response and
	// echoed on subsequent requests. Optional per spec — many servers
	// don't issue one.
	sessionID string
}

// NewClient builds an MCP client pointing at endpoint, with a sensible
// default timeout. Replace HTTP for finer control.
func NewClient(endpoint string) *Client {
	return &Client{
		Endpoint: endpoint,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Initialize performs the MCP handshake. Required before tools/list or
// tools/call against most servers, even if our implementation is
// stateless.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "krypton",
			"version": "0.1.0",
		},
	}
	var out InitializeResult
	if err := c.call(ctx, "initialize", params, &out); err != nil {
		return nil, err
	}
	// Fire-and-forget notification that the client is ready.
	_ = c.notify(ctx, "notifications/initialized", nil)
	return &out, nil
}

// ListTools fetches the tools the server exposes.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var result ListToolsResult
	if err := c.call(ctx, "tools/list", nil, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool invokes a tool. args is a JSON object encoding the tool's
// inputs as documented by Tool.InputSchema.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (*CallToolResult, error) {
	params := CallToolParams{Name: name, Arguments: args}
	var result CallToolResult
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// call issues a JSON-RPC request and decodes the result into out (if non-nil).
func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal %s params: %w", method, err)
		}
		raw = b
	}
	req := Request{
		JSONRPC: JSONRPCVersion,
		ID:      int(c.id.Add(1)),
		Method:  method,
		Params:  raw,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Accept text/event-stream so a server is free to upgrade to SSE for
	// long-running calls; for our tools/list and tools/call paths the
	// servers we talk to always respond with plain JSON.
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return fmt.Errorf("post %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" && c.sessionID == "" {
		c.sessionID = sid
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", method, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp %s: HTTP %d %s", method, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// Streamable HTTP sometimes wraps the JSON-RPC response in SSE
	// `data:` lines. Tolerate either form.
	payload := decodeSSEOrJSON(respBody)

	var rpcResp Response
	if err := json.Unmarshal(payload, &rpcResp); err != nil {
		return fmt.Errorf("decode %s response: %w (body=%q)", method, err, payload)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("mcp %s error %d: %s", method, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if out != nil {
		if len(rpcResp.Result) == 0 {
			return errors.New("mcp " + method + ": empty result")
		}
		if err := json.Unmarshal(rpcResp.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}

// notify sends a fire-and-forget notification (no response expected).
func (c *Client) notify(ctx context.Context, method string, params any) error {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = b
	}
	body, _ := json.Marshal(Notification{JSONRPC: JSONRPCVersion, Method: method, Params: raw})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// decodeSSEOrJSON returns the payload — either the body verbatim (if
// it parses as JSON) or the first `data: ...` line concatenated.
func decodeSSEOrJSON(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed
	}
	var buf bytes.Buffer
	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("data:")) {
			buf.Write(bytes.TrimSpace(line[len("data:"):]))
		}
	}
	if buf.Len() == 0 {
		return trimmed
	}
	return buf.Bytes()
}
