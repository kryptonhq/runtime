//go:build envtest

/*
Copyright 2026 Krypton Authors.
*/

// Package integration runs Krypton's controllers against a real
// kube-apiserver + etcd via controller-runtime's envtest.
//
// What this tier catches that the fake-client unit tests structurally
// cannot:
//
//   - OpenAPI schema validation, defaulting, and pruning of unknown fields.
//     Krypton's CRDs are hand-written (see the
//     `controller-gen.kubebuilder.io/version: hand-written-bootstrap`
//     annotation), so a field added to the Go types with no matching
//     property in the CRD is silently dropped on write. The fake client
//     does no schema enforcement at all and stays green.
//   - Status subresource semantics — spec writes cannot change status and
//     vice versa.
//   - Optimistic concurrency (resourceVersion conflicts).
//   - SetupWithManager actually wiring up watches.
//
// What envtest deliberately does NOT provide, per the upstream docs, and
// which therefore must never be asserted here:
//
//   - kubelet: Pods are never scheduled and never become Ready.
//   - built-in controllers: a Deployment creates no ReplicaSet, a
//     ReplicaSet creates no Pod, and Deployment.status stays zero.
//   - garbage collector: OwnerReferences do not cascade deletes. Assert
//     that the reference is SET, never that the child disappears.
//
// Run with: make test-envtest
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/controller"
)

var (
	// k8sClient talks to the envtest API server directly, bypassing the
	// manager's cache. Reads are therefore always fresh.
	k8sClient client.Client
	testEnv   *envtest.Environment
	cfg       *rest.Config
	scheme    *runtime.Scheme

	// cancelManager stops the background manager during teardown.
	cancelManager context.CancelFunc
)

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration suite setup failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stderr)))

	scheme = runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return 0, fmt.Errorf("client-go scheme: %w", err)
	}
	if err := kryptonv1alpha1.AddToScheme(scheme); err != nil {
		return 0, fmt.Errorf("krypton scheme: %w", err)
	}

	// Load the real, committed CRDs. Using config/crd/bases (rather than a
	// scheme-only setup) is the whole point: it makes the API server enforce
	// the same schema operators get from the Helm chart.
	crdPath, err := filepath.Abs(filepath.Join("..", "..", "config", "crd", "bases"))
	if err != nil {
		return 0, err
	}
	if _, err := os.Stat(crdPath); err != nil {
		return 0, fmt.Errorf("CRD directory %s: %w", crdPath, err)
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{crdPath},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testEnv.Start()
	if err != nil {
		return 0, fmt.Errorf("start envtest (is KUBEBUILDER_ASSETS set? try `make envtest`): %w", err)
	}
	defer func() {
		if cancelManager != nil {
			cancelManager()
		}
		if stopErr := testEnv.Stop(); stopErr != nil {
			fmt.Fprintf(os.Stderr, "envtest stop: %v\n", stopErr)
		}
	}()

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return 0, fmt.Errorf("build client: %w", err)
	}

	if err := startManager(); err != nil {
		return 0, fmt.Errorf("start manager: %w", err)
	}

	return m.Run(), nil
}

// startManager runs both reconcilers through a real manager, which is what
// exercises SetupWithManager (unreachable from unit tests) and the watch
// plumbing behind it.
func startManager() error {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		// Disable the metrics listener; parallel packages would fight over
		// the port and the metrics are asserted in internal/metrics.
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	if err != nil {
		return err
	}

	if err := (&controller.AgentReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		ProxyImage: "ghcr.io/kryptonhq/krypton-proxy:test",
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("agent controller: %w", err)
	}

	if err := (&controller.ModelReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("model controller: %w", err)
	}

	var ctx context.Context
	ctx, cancelManager = context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Start(ctx) }()

	// Block until the cache has synced, otherwise the first test can race
	// the manager's startup and see an empty world.
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("cache failed to sync")
	}

	select {
	case err := <-errCh:
		return fmt.Errorf("manager exited during startup: %w", err)
	case <-time.After(100 * time.Millisecond):
	}
	return nil
}

// ---- helpers ---------------------------------------------------------------

const (
	// The manager reconciles asynchronously, so assertions poll. These are
	// generous enough for a loaded CI runner but still fail fast locally.
	eventuallyTimeout  = 20 * time.Second
	eventuallyInterval = 100 * time.Millisecond
)

// eventually polls cond until it returns nil or the timeout elapses. On
// timeout it fails the test with the last error, which keeps the useful
// detail instead of reporting a bare "timed out".
func eventually(t *testing.T, desc string, cond func() error) {
	t.Helper()
	deadline := time.Now().Add(eventuallyTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = cond()
		if lastErr == nil {
			return
		}
		time.Sleep(eventuallyInterval)
	}
	t.Fatalf("timed out after %s waiting for %s: %v", eventuallyTimeout, desc, lastErr)
}

// consistently asserts cond holds for a short settling window. Used to
// prove the controller does NOT do something (e.g. keep rewriting status).
func consistently(t *testing.T, desc string, window time.Duration, cond func() error) {
	t.Helper()
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		if err := cond(); err != nil {
			t.Fatalf("%s did not hold: %v", desc, err)
		}
		time.Sleep(eventuallyInterval)
	}
}

// newNamespace creates a uniquely named namespace per test so tests can run
// in any order without colliding on object names.
func newNamespace(t *testing.T) string {
	t.Helper()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "krypton-it-",
		},
	}
	if err := k8sClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	// Namespace deletion in envtest never completes (no namespace
	// controller), so there is nothing useful to clean up here.
	return ns.Name
}
