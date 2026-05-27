/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package sidecar implements krypton-proxy, the per-pod sidecar injected
// next to every Agent container. It enforces concurrency, surfaces
// Prometheus metrics, reports in-flight counts to the activator, and
// drives pod self-termination on idle (serverless mode).
package sidecar

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Mode mirrors the Agent CR's runtime mode.
type Mode string

const (
	ModeServerless Mode = "serverless"
	ModeAlwaysOn   Mode = "always-on"
)

// Config is parsed from environment variables injected by the reconciler.
// It's a separate value type so tests can construct it directly.
type Config struct {
	// AgentName and AgentNamespace tag metrics and logs.
	AgentName      string
	AgentNamespace string

	// ListenAddr is where the sidecar accepts traffic.
	ListenAddr string

	// UpstreamURL is the address of the user container in the same pod.
	UpstreamURL string

	// Concurrency caps in-flight requests; over the cap returns 503.
	Concurrency int

	// Mode selects serverless (self-terminate on idle) or always-on.
	Mode Mode

	// IdleTimeout: serverless pods exit after this much inactivity.
	IdleTimeout time.Duration

	// ShutdownTimeout bounds graceful drain on SIGTERM.
	ShutdownTimeout time.Duration
}

// ConfigFromEnv reads sidecar configuration from the process environment.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		AgentName:       getenv("KRYPTON_AGENT_NAME", "unknown"),
		AgentNamespace:  getenv("KRYPTON_AGENT_NAMESPACE", "default"),
		ListenAddr:      getenv("KRYPTON_LISTEN_ADDR", ":8888"),
		UpstreamURL:     getenv("KRYPTON_UPSTREAM_URL", "http://127.0.0.1:8080"),
		Mode:            Mode(getenv("KRYPTON_MODE", string(ModeServerless))),
		ShutdownTimeout: 25 * time.Second,
	}

	concurrency, err := strconv.Atoi(getenv("KRYPTON_CONCURRENCY", "8"))
	if err != nil || concurrency < 1 {
		return Config{}, fmt.Errorf("invalid KRYPTON_CONCURRENCY: %q", os.Getenv("KRYPTON_CONCURRENCY"))
	}
	cfg.Concurrency = concurrency

	idleStr := getenv("KRYPTON_IDLE_TIMEOUT", "300s")
	idle, err := time.ParseDuration(idleStr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid KRYPTON_IDLE_TIMEOUT %q: %w", idleStr, err)
	}
	cfg.IdleTimeout = idle

	if shutStr := os.Getenv("KRYPTON_SHUTDOWN_TIMEOUT"); shutStr != "" {
		d, err := time.ParseDuration(shutStr)
		if err != nil {
			return Config{}, fmt.Errorf("invalid KRYPTON_SHUTDOWN_TIMEOUT %q: %w", shutStr, err)
		}
		cfg.ShutdownTimeout = d
	}

	if cfg.Mode != ModeServerless && cfg.Mode != ModeAlwaysOn {
		return Config{}, fmt.Errorf("invalid KRYPTON_MODE: %q", cfg.Mode)
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
