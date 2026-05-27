/*
Copyright 2026 Krypton Authors.
*/

package scaler

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

// TestDecide_KeepsWarmDuringIdleWindow guards against the flapping bug
// where, between requests, the scaler would race the activator to zero.
func TestDecide_KeepsWarmDuringIdleWindow(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	d := Decider{Now: func() time.Time { return now }}
	a := agentFor(t, func(a *kryptonv1alpha1.Agent) {
		// 30s ago — well inside the 5min default idle window.
		a.Status.LastInvocationAt = &metav1.Time{Time: now.Add(-30 * time.Second)}
		a.Spec.MinReplicas = 0
	})
	got := d.Decide(Input{Agent: a, Inflight: 0})
	if got != 1 {
		t.Errorf("serverless recently-invoked + no load = %d, want 1 (warm)", got)
	}
	if a.Spec.Mode != kryptonv1alpha1.ModeServerless {
		t.Fatal("test misconfigured")
	}
}
