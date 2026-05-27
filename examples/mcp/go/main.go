/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Command mcp-hello is a tiny Model Context Protocol server used to
// smoke-test Krypton's MCP support. Three toy tools: echo, add, time.
// No external dependencies, no LLM calls.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const protocolVersion = "2025-06-18"

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // nil for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var tools = []tool{
	{
		Name:        "echo",
		Description: "Echo back the input message.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"message"},
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
	},
	{
		Name:        "add",
		Description: "Add two numbers and return the sum.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"a", "b"},
			"properties": map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
		},
	},
	{
		Name:        "time",
		Description: "Return the current UTC time as ISO-8601.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           http.HandlerFunc(rpc),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("mcp-hello listening", "port", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rpc(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Health probe / browser hit.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"server":"mcp-hello","status":"ok"}`))
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req rpcReq
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Notifications carry no id; acknowledge and move on.
	if len(req.ID) == 0 || strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := rpcResp{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "mcp-hello",
				"version": "0.1.0",
			},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": tools}
	case "tools/call":
		resp.Result, resp.Error = handleCall(req.Params)
	default:
		resp.Error = &rpcErr{Code: -32601, Message: "method not found: " + req.Method}
	}

	writeRPCResponse(w, r, resp)
}

// writeRPCResponse honors the MCP streamable-HTTP transport: if the
// client sends `Accept: text/event-stream` it gets a single `data:`
// SSE frame, otherwise plain JSON.
func writeRPCResponse(w http.ResponseWriter, r *http.Request, resp rpcResp) {
	body, _ := json.Marshal(resp)
	if acceptsEventStream(r) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message\ndata: "))
		_, _ = w.Write(body)
		_, _ = w.Write([]byte("\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func acceptsEventStream(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func handleCall(params json.RawMessage) (any, *rpcErr) {
	var p callParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcErr{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	switch p.Name {
	case "echo":
		var args struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(p.Arguments, &args); err != nil {
			return nil, &rpcErr{Code: -32602, Message: err.Error()}
		}
		return textResult(args.Message), nil
	case "add":
		var args struct {
			A float64 `json:"a"`
			B float64 `json:"b"`
		}
		if err := json.Unmarshal(p.Arguments, &args); err != nil {
			return nil, &rpcErr{Code: -32602, Message: err.Error()}
		}
		return textResult(fmt.Sprintf("%v", args.A+args.B)), nil
	case "time":
		return textResult(time.Now().UTC().Format(time.RFC3339)), nil
	default:
		return nil, &rpcErr{Code: -32601, Message: "unknown tool: " + p.Name}
	}
}

func textResult(s string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": s},
		},
	}
}
