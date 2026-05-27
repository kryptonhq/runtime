/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Command manager runs the Krypton controller, admission webhooks, and
// the scaling decider.
package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/controller"
	"github.com/kryptonhq/runtime/internal/scaler"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kryptonv1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
		enableWebhooks       bool
		proxyImage           string
		enableScaler         bool
		scalerIntervalMs     int
		scalerStableWindowMs int
		sidecarPort          int
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "address for the metrics endpoint")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "address for /healthz and /readyz")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "enable leader election")
	flag.BoolVar(&enableWebhooks, "enable-webhooks", true, "register admission webhooks")
	flag.StringVar(&proxyImage, "proxy-image", controller.DefaultProxyImage, "krypton-proxy sidecar image")
	flag.BoolVar(&enableScaler, "enable-scaler", true, "run the scaling decider alongside the controller")
	flag.IntVar(&scalerIntervalMs, "scaler-interval-ms", 1000, "scaling decider tick interval")
	flag.IntVar(&scalerStableWindowMs, "scaler-stable-window-ms", 60000, "scale-down hysteresis window after each scale-up")
	flag.IntVar(&sidecarPort, "sidecar-port", 8888, "krypton-proxy sidecar HTTP port; must match controller injection")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setup := ctrl.Log.WithName("setup")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "krypton-runtime-leader.krypton.ai",
	})
	if err != nil {
		setup.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&controller.AgentReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		ProxyImage: proxyImage,
	}).SetupWithManager(mgr); err != nil {
		setup.Error(err, "unable to set up Agent controller")
		os.Exit(1)
	}

	if err := (&controller.ModelReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setup.Error(err, "unable to set up Model controller")
		os.Exit(1)
	}

	if enableWebhooks {
		if err := (&kryptonv1alpha1.Agent{}).SetupWebhookWithManager(mgr); err != nil {
			setup.Error(err, "unable to set up Agent webhooks")
			os.Exit(1)
		}
	}

	if enableScaler {
		sc := &scaler.Scaler{
			Client:   mgr.GetClient(),
			Probe:    scaler.NewHTTPLoadProbe(mgr.GetClient(), sidecarPort),
			Interval: time.Duration(scalerIntervalMs) * time.Millisecond,
			Decider: scaler.Decider{
				StableWindow: time.Duration(scalerStableWindowMs) * time.Millisecond,
			},
		}
		if err := mgr.Add(sc); err != nil {
			setup.Error(err, "unable to add scaler runnable")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		setup.Error(err, "healthz")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		setup.Error(err, "readyz")
		os.Exit(1)
	}

	setup.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setup.Error(err, "manager exited")
		os.Exit(1)
	}
}
