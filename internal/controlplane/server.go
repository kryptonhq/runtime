/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controlplane

import (
	"context"
	"errors"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ServerOptions configures the public HTTP server.
type ServerOptions struct {
	ListenAddr      string
	ShutdownTimeout time.Duration
}

// Run starts the public HTTP server and blocks until ctx is cancelled or
// the server fails. It performs a graceful shutdown bounded by
// opts.ShutdownTimeout.
func Run(ctx context.Context, api *API, opts ServerOptions) error {
	if opts.ListenAddr == "" {
		opts.ListenAddr = ":8090"
	}
	if opts.ShutdownTimeout == 0 {
		opts.ShutdownTimeout = 25 * time.Second
	}

	logger := log.FromContext(ctx).WithValues("component", "control-plane")
	srv := &http.Server{
		Addr:              opts.ListenAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", opts.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
