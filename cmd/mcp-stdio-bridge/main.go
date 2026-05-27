/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Command mcp-stdio-bridge wraps a stdio MCP server in HTTP so it can
// run as a Krypton agent (Krypton only deploys network services). The
// child process speaks JSON-RPC over its stdin/stdout, framed as
// newline-delimited JSON; we expose the same JSON-RPC endpoint over
// HTTP POST.
//
// Usage: set MCP_COMMAND in the container env, e.g.
//
//	env:
//	  - name: MCP_COMMAND
//	    value: "npx -y @modelcontextprotocol/server-filesystem /data"
//
// The bridge forks once at startup and reuses the child for every
// HTTP request. JSON-RPC IDs are matched so concurrent requests don't
// cross-talk. One-shot stdio servers (process-per-call) are not
// supported.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type bridge struct {
	mu     sync.Mutex
	stdin  io.WriteCloser
	stdout *bufio.Reader

	pendingMu sync.Mutex
	pending   map[string]chan []byte
}

func main() {
	command := os.Getenv("MCP_COMMAND")
	if command == "" {
		fmt.Fprintln(os.Stderr, "MCP_COMMAND env var is required")
		os.Exit(2)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		fmt.Fprintln(os.Stderr, "MCP_COMMAND parsed to zero tokens")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		die(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		die(err)
	}
	if err := cmd.Start(); err != nil {
		die(fmt.Errorf("start MCP child: %w", err))
	}
	slog.Info("started MCP child", "command", command, "pid", cmd.Process.Pid)

	br := &bridge{
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		pending: make(map[string]chan []byte),
	}
	go br.readLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", br.handle)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		_ = stdin.Close()
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
	}()

	slog.Info("mcp-stdio-bridge listening", "port", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		die(err)
	}
}

// readLoop pulls JSON-RPC responses off the child's stdout and routes
// each by id to the waiting HTTP handler.
func (b *bridge) readLoop() {
	for {
		line, err := b.stdout.ReadBytes('\n')
		if len(line) > 0 {
			b.dispatch(line)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				slog.Warn("child stdout closed")
				return
			}
			slog.Error("read child stdout", "error", err)
			return
		}
	}
}

func (b *bridge) dispatch(line []byte) {
	var probe struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(line, &probe); err != nil || len(probe.ID) == 0 {
		// Notification or unparseable line — drop it. Server→client
		// notifications aren't routable to a specific HTTP request.
		return
	}
	key := string(probe.ID)
	b.pendingMu.Lock()
	ch, ok := b.pending[key]
	if ok {
		delete(b.pending, key)
	}
	b.pendingMu.Unlock()
	if ok {
		ch <- line
	}
}

// handle proxies one HTTP POST to the child and waits for its response.
// Notifications (no id) are sent fire-and-forget.
func (b *bridge) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var probe struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(body, &probe)
	isNotification := len(probe.ID) == 0

	if !strings.HasSuffix(string(body), "\n") {
		body = append(body, '\n')
	}

	// Serialize writes to the child's stdin; reads are demuxed by id.
	b.mu.Lock()
	if _, err := b.stdin.Write(body); err != nil {
		b.mu.Unlock()
		http.Error(w, "write child: "+err.Error(), http.StatusBadGateway)
		return
	}
	b.mu.Unlock()

	if isNotification {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	key := string(probe.ID)
	ch := make(chan []byte, 1)
	b.pendingMu.Lock()
	b.pending[key] = ch
	b.pendingMu.Unlock()

	select {
	case line := <-ch:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(line)
	case <-r.Context().Done():
		b.pendingMu.Lock()
		delete(b.pending, key)
		b.pendingMu.Unlock()
		http.Error(w, "client cancelled", http.StatusGatewayTimeout)
	case <-time.After(2 * time.Minute):
		b.pendingMu.Lock()
		delete(b.pending, key)
		b.pendingMu.Unlock()
		http.Error(w, "no response from child", http.StatusGatewayTimeout)
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
