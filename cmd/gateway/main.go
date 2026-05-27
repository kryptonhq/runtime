/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Command gateway is the Krypton public ingress. See internal/gateway for
// routing + activation logic.
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/gateway"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kryptonv1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		listenAddr        string
		metricsAddr       string
		probeAddr         string
		maxBufferPerAgent int
		pollIntervalMs    int
		defaultStartupMs  int
	)
	flag.StringVar(&listenAddr, "listen-address", ":8080", "address for the public HTTP endpoint")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8081", "address for /metrics (controller-runtime)")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8082", "address for /healthz, /readyz (controller-runtime)")
	flag.IntVar(&maxBufferPerAgent, "max-buffer-per-agent", 100, "max concurrent cold-start waiters per agent before 503")
	flag.IntVar(&pollIntervalMs, "poll-interval-ms", 50, "endpoint readiness poll interval during cold start")
	flag.IntVar(&defaultStartupMs, "default-startup-timeout-ms", 30000, "fallback cold-start timeout when spec.startupTimeout is 0")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setup := ctrl.Log.WithName("setup")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), manager.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		setup.Error(err, "unable to start manager")
		os.Exit(1)
	}

	gw := &gateway.Gateway{
		Activator: &gateway.Activator{
			Client:                mgr.GetClient(),
			MaxBufferPerAgent:     maxBufferPerAgent,
			PollInterval:          time.Duration(pollIntervalMs) * time.Millisecond,
			DefaultStartupTimeout: time.Duration(defaultStartupMs) * time.Millisecond,
		},
	}

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           gw.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx := ctrl.SetupSignalHandler()

	go func() {
		setup.Info("gateway listening", "addr", listenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			setup.Error(err, "http server")
			os.Exit(1)
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	setup.Info("starting manager cache")
	if err := mgr.Start(ctx); err != nil {
		setup.Error(err, "manager exited")
		os.Exit(1)
	}
}
