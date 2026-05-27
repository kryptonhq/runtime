/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Command krypton-proxy is the per-pod sidecar injected next to every Agent
// container. See internal/sidecar for the bulk of the logic.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kryptonhq/runtime/internal/sidecar"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := sidecar.ConfigFromEnv()
	if err != nil {
		logger.Error("invalid sidecar config", "error", err)
		os.Exit(2)
	}
	logger = logger.With("agent", cfg.AgentName, "namespace", cfg.AgentNamespace)

	proxy, err := sidecar.NewProxy(cfg)
	if err != nil {
		logger.Error("init proxy", "error", err)
		os.Exit(2)
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           proxy.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go runIdleWatcher(rootCtx, logger, proxy, cancel)

	go func() {
		logger.Info("sidecar listening", "addr", cfg.ListenAddr, "upstream", cfg.UpstreamURL, "concurrency", cfg.Concurrency)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server", "error", err)
			cancel()
		}
	}()

	<-rootCtx.Done()
	logger.Info("shutdown requested, draining")
	proxy.MarkShutdown()

	drainCtx, drainCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer drainCancel()
	if err := proxy.DrainAndShutdown(drainCtx); err != nil {
		logger.Warn("drain ended with error", "error", err, "inflight", proxy.Inflight())
	}
	if err := srv.Shutdown(drainCtx); err != nil {
		logger.Warn("http shutdown", "error", err)
	}
	logger.Info("bye")
}

// runIdleWatcher cancels the root context once the proxy reports idle so the
// pod can self-terminate. Always-on mode short-circuits in IsIdle.
func runIdleWatcher(ctx context.Context, logger *slog.Logger, proxy *sidecar.Proxy, cancel context.CancelFunc) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if proxy.IsIdle(time.Now()) {
				logger.Info("idle timeout elapsed, exiting")
				cancel()
				return
			}
		}
	}
}
