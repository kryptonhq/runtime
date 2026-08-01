/*
Copyright 2026 Krypton Authors.
*/

package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

// modelStatusEqual decides whether the reconciler issues a status update.
// A false negative means a write on every reconcile (hot loop against the
// API server); a false positive means status silently goes stale.
func TestModelStatusEqual(t *testing.T) {
	base := func() *kryptonv1alpha1.ModelStatus {
		return &kryptonv1alpha1.ModelStatus{
			Phase:              kryptonv1alpha1.ModelPhaseReady,
			Replicas:           2,
			ReadyReplicas:      2,
			URL:                "http://qwen.models.svc:8080",
			ObservedGeneration: 3,
			Conditions: []metav1.Condition{{
				Type:   kryptonv1alpha1.ConditionAvailable,
				Status: metav1.ConditionTrue,
				Reason: "MinimumReplicasAvailable",
			}},
		}
	}

	tests := []struct {
		name   string
		mutate func(*kryptonv1alpha1.ModelStatus)
		want   bool
	}{
		{
			name:   "identical",
			mutate: func(*kryptonv1alpha1.ModelStatus) {},
			want:   true,
		},
		{
			name:   "phase differs",
			mutate: func(s *kryptonv1alpha1.ModelStatus) { s.Phase = kryptonv1alpha1.ModelPhasePending },
			want:   false,
		},
		{
			name:   "replicas differ",
			mutate: func(s *kryptonv1alpha1.ModelStatus) { s.Replicas = 3 },
			want:   false,
		},
		{
			name:   "readyReplicas differ",
			mutate: func(s *kryptonv1alpha1.ModelStatus) { s.ReadyReplicas = 1 },
			want:   false,
		},
		{
			name:   "url differs",
			mutate: func(s *kryptonv1alpha1.ModelStatus) { s.URL = "http://elsewhere:8080" },
			want:   false,
		},
		{
			name:   "observedGeneration differs",
			mutate: func(s *kryptonv1alpha1.ModelStatus) { s.ObservedGeneration = 4 },
			want:   false,
		},
		{
			name:   "condition count differs",
			mutate: func(s *kryptonv1alpha1.ModelStatus) { s.Conditions = nil },
			want:   false,
		},
		{
			name:   "condition type differs",
			mutate: func(s *kryptonv1alpha1.ModelStatus) { s.Conditions[0].Type = kryptonv1alpha1.ConditionProgressing },
			want:   false,
		},
		{
			name:   "condition status differs",
			mutate: func(s *kryptonv1alpha1.ModelStatus) { s.Conditions[0].Status = metav1.ConditionFalse },
			want:   false,
		},
		{
			name:   "condition reason differs",
			mutate: func(s *kryptonv1alpha1.ModelStatus) { s.Conditions[0].Reason = "Whatever" },
			want:   false,
		},
		{
			// Message and LastTransitionTime are deliberately excluded from
			// the comparison: a changing timestamp would make every
			// reconcile look like a real change and write-loop.
			name: "condition message and timestamp are ignored",
			mutate: func(s *kryptonv1alpha1.ModelStatus) {
				s.Conditions[0].Message = "totally different prose"
				s.Conditions[0].LastTransitionTime = metav1.Now()
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, b := base(), base()
			tc.mutate(b)
			if got := modelStatusEqual(a, b); got != tc.want {
				t.Errorf("modelStatusEqual() = %v, want %v", got, tc.want)
			}
			// The comparison must be symmetric, or reconcile behaviour would
			// depend on argument order.
			if got := modelStatusEqual(b, a); got != tc.want {
				t.Errorf("modelStatusEqual() reversed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestModelStatusEqualEmptyStatuses(t *testing.T) {
	// A freshly created Model has a zero status; comparing two zero values
	// must be equal so the first reconcile doesn't write a no-op update.
	if !modelStatusEqual(&kryptonv1alpha1.ModelStatus{}, &kryptonv1alpha1.ModelStatus{}) {
		t.Error("two zero-valued statuses should compare equal")
	}
}
