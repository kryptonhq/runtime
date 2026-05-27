/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Command control-plane runs the Krypton control plane HTTP API.
//
// It boots a controller-runtime manager that maintains an informer-backed
// cache of Agent CRs, then serves the REST API from that cache.
//
// Persistence: if DATABASE_URL (or --database-url) is set, agents are
// mirrored into Postgres on every reconcile via the store Syncer. The
// API still serves from the informer cache — the mirror is for offline
// querying and (with M6/M8) joining against invocation and metrics
// history. When the flag is empty, an in-memory store stands in.
package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/controlplane"
	"github.com/kryptonhq/runtime/internal/controlplane/store"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kryptonv1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		apiAddr     string
		metricsAddr string
		probeAddr   string
		databaseURL string
	)
	flag.StringVar(&apiAddr, "api-bind-address", ":8090", "address for the public REST API")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8091", "address for /metrics (controller-runtime)")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8092", "address for /healthz, /readyz (controller-runtime)")
	flag.StringVar(&databaseURL, "database-url", os.Getenv("DATABASE_URL"), "postgres DSN; if empty, an in-memory store is used")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setup := ctrl.Log.WithName("setup")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), manager.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		// No leader election — the control plane is stateless and
		// multiple replicas can serve traffic concurrently.
	})
	if err != nil {
		setup.Error(err, "unable to start manager")
		os.Exit(1)
	}

	api := &controlplane.API{Client: mgr.GetClient()}
	ctx := ctrl.SetupSignalHandler()

	var registry store.Store
	if databaseURL != "" {
		pg, err := store.NewPostgres(ctx, databaseURL)
		if err != nil {
			setup.Error(err, "unable to open postgres store")
			os.Exit(1)
		}
		defer func() { _ = pg.Close() }()
		registry = pg
		setup.Info("using postgres registry mirror")
	} else {
		setup.Info("DATABASE_URL not set, using in-memory registry mirror")
		registry = store.NewMemory()
	}

	if err := (&controlplane.Syncer{
		Client: mgr.GetClient(),
		Store:  registry,
	}).SetupWithManager(mgr); err != nil {
		setup.Error(err, "unable to start syncer")
		os.Exit(1)
	}

	go func() {
		if err := controlplane.Run(ctx, api, controlplane.ServerOptions{ListenAddr: apiAddr}); err != nil {
			setup.Error(err, "api server exited")
			os.Exit(1)
		}
	}()

	setup.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setup.Error(err, "manager exited")
		os.Exit(1)
	}
}
