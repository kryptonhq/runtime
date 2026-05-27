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

func agentFor(t *testing.T, mut func(*kryptonv1alpha1.Agent)) *kryptonv1alpha1.Agent {
	t.Helper()
	a := &kryptonv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "travel", Namespace: "agents"},
		Spec: kryptonv1alpha1.AgentSpec{
			Mode:             kryptonv1alpha1.ModeServerless,
			Concurrency:      8,
			MinReplicas:      0,
			MaxReplicas:      10,
			ScaleToZeroAfter: metav1.Duration{Duration: 5 * time.Minute},
		},
	}
	if mut != nil {
		mut(a)
	}
	return a
}

func TestDecide_ConcurrencyFormula(t *testing.T) {
	d := Decider{}
	cases := []struct {
		name     string
		inflight int
		want     int32
	}{
		// inflight=0 is exercised by TestDecide_KeepsWarmDuringIdleWindow;
		// the formula here covers the active path only.
		{"one", 1, 1},
		{"exactly one bucket", 8, 1},
		{"one over", 9, 2},
		{"two full buckets", 16, 2},
		{"clamped to max", 1000, 10},
	}
	now := time.Now()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := agentFor(t, func(a *kryptonv1alpha1.Agent) {
				// Force "not idle" so the formula is what gets returned.
				a.Status.LastInvocationAt = &metav1.Time{Time: now}
			})
			got := d.Decide(Input{Agent: a, Inflight: tc.inflight})
			if got != tc.want {
				t.Errorf("inflight=%d → desired=%d, want %d", tc.inflight, got, tc.want)
			}
		})
	}
}

func TestDecide_HonorsMinReplicas(t *testing.T) {
	d := Decider{}
	a := agentFor(t, func(a *kryptonv1alpha1.Agent) {
		a.Spec.MinReplicas = 2
		a.Status.LastInvocationAt = &metav1.Time{Time: time.Now()}
	})
	got := d.Decide(Input{Agent: a, Inflight: 1})
	if got != 2 {
		t.Errorf("got %d, want 2 (min floor)", got)
	}
}

func TestDecide_ServerlessIdleScalesToZero(t *testing.T) {
	d := Decider{Now: func() time.Time { return time.Unix(1_000_000, 0) }}
	a := agentFor(t, func(a *kryptonv1alpha1.Agent) {
		a.Spec.MinReplicas = 0
		// Last invocation well outside the idle window.
		a.Status.LastInvocationAt = &metav1.Time{Time: time.Unix(0, 0)}
	})
	if got := d.Decide(Input{Agent: a, Inflight: 0}); got != 0 {
		t.Errorf("idle serverless should scale to 0, got %d", got)
	}
}

func TestDecide_ServerlessRecentInvocationHoldsMin(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	d := Decider{Now: func() time.Time { return now }}
	a := agentFor(t, func(a *kryptonv1alpha1.Agent) {
		a.Spec.MinReplicas = 1
		a.Status.LastInvocationAt = &metav1.Time{Time: now.Add(-30 * time.Second)}
	})
	got := d.Decide(Input{Agent: a, Inflight: 0})
	if got != 1 {
		t.Errorf("not idle yet, got %d, want 1 (min)", got)
	}
}

func TestDecide_AlwaysOnNeverScalesBelowOne(t *testing.T) {
	d := Decider{}
	a := agentFor(t, func(a *kryptonv1alpha1.Agent) {
		a.Spec.Mode = kryptonv1alpha1.ModeAlwaysOn
		a.Spec.MinReplicas = 0
	})
	got := d.Decide(Input{Agent: a, Inflight: 0})
	if got != 1 {
		t.Errorf("always-on with no load = %d, want 1", got)
	}
}

func TestDecide_NeverInvokedAndNoLoadStaysAtZero(t *testing.T) {
	d := Decider{}
	a := agentFor(t, nil) // LastInvocationAt nil
	got := d.Decide(Input{Agent: a, Inflight: 0})
	if got != 0 {
		t.Errorf("never-invoked serverless with no load = %d, want 0", got)
	}
}

func TestDecide_HysteresisBlocksScaleDown(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	d := Decider{
		StableWindow: time.Minute,
		Now:          func() time.Time { return now },
	}
	a := agentFor(t, func(a *kryptonv1alpha1.Agent) {
		a.Status.DesiredReplicas = 5
		a.Status.LastInvocationAt = &metav1.Time{Time: now}
	})
	// Load has dropped — the formula would say 1.
	in := Input{
		Agent:       a,
		Inflight:    1,
		LastScaleUp: now.Add(-10 * time.Second), // recent
	}
	if got := d.Decide(in); got != 5 {
		t.Errorf("hysteresis didn't block scale-down: got %d, want 5", got)
	}

	// Same inputs but with last scale-up older than the window.
	in.LastScaleUp = now.Add(-2 * time.Minute)
	if got := d.Decide(in); got != 1 {
		t.Errorf("after stable window: got %d, want 1", got)
	}
}

func TestDecide_HysteresisAllowsScaleUp(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	d := Decider{
		StableWindow: time.Minute,
		Now:          func() time.Time { return now },
	}
	a := agentFor(t, func(a *kryptonv1alpha1.Agent) {
		a.Status.DesiredReplicas = 2
		a.Status.LastInvocationAt = &metav1.Time{Time: now}
	})
	in := Input{
		Agent:       a,
		Inflight:    40, // 40/8 = 5
		LastScaleUp: now.Add(-1 * time.Second),
	}
	if got := d.Decide(in); got != 5 {
		t.Errorf("hysteresis should not block scale-up: got %d, want 5", got)
	}
}
