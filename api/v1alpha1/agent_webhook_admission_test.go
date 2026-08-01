/*
Copyright 2026 Krypton Authors.
*/

package v1alpha1

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ValidateUpdate runs the same invariants as create. Without this, an
// Agent could be created valid and then edited into an invalid state.
func TestValidateUpdate(t *testing.T) {
	ctx := context.Background()
	v := &AgentValidator{}

	t.Run("accepts a valid update", func(t *testing.T) {
		old := newValidAgent()
		updated := newValidAgent()
		updated.Spec.MaxReplicas = 20

		warnings, err := v.ValidateUpdate(ctx, old, updated)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(warnings) != 0 {
			t.Errorf("warnings = %v, want none", warnings)
		}
	})

	t.Run("rejects an update that clears the image", func(t *testing.T) {
		old := newValidAgent()
		updated := newValidAgent()
		updated.Spec.Image = ""

		if _, err := v.ValidateUpdate(ctx, old, updated); err == nil {
			t.Fatal("expected an error when image is cleared")
		} else if !strings.Contains(err.Error(), "image is required") {
			t.Errorf("error = %v, want it to mention image", err)
		}
	})

	t.Run("rejects an update that inverts the replica bounds", func(t *testing.T) {
		old := newValidAgent()
		updated := newValidAgent()
		updated.Spec.MinReplicas = 9
		updated.Spec.MaxReplicas = 2

		if _, err := v.ValidateUpdate(ctx, old, updated); err == nil {
			t.Fatal("expected an error when maxReplicas < minReplicas")
		}
	})

	t.Run("rejects a non-Agent object", func(t *testing.T) {
		_, err := v.ValidateUpdate(ctx, newValidAgent(), &corev1.Pod{})
		if err == nil {
			t.Fatal("expected an error for a non-Agent object")
		}
		if !apierrors.IsBadRequest(err) {
			t.Errorf("error = %v, want a BadRequest", err)
		}
	})
}

// Delete is unconditionally allowed — there are no finalizer-style
// invariants to enforce on teardown.
func TestValidateDelete(t *testing.T) {
	ctx := context.Background()
	v := &AgentValidator{}

	warnings, err := v.ValidateDelete(ctx, newValidAgent())
	if err != nil {
		t.Fatalf("ValidateDelete returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	// Even a nonsense object is fine; nothing is inspected.
	if _, err := v.ValidateDelete(ctx, &corev1.Pod{}); err != nil {
		t.Errorf("ValidateDelete on a non-Agent returned error: %v", err)
	}
}

func TestValidateCreateRejectsNonAgent(t *testing.T) {
	_, err := (&AgentValidator{}).ValidateCreate(context.Background(), &corev1.Pod{})
	if err == nil {
		t.Fatal("expected an error for a non-Agent object")
	}
	if !apierrors.IsBadRequest(err) {
		t.Errorf("error = %v, want a BadRequest", err)
	}
}

func TestDefaultRejectsNonAgent(t *testing.T) {
	err := (&AgentDefaulter{}).Default(context.Background(), &corev1.Pod{})
	if err == nil {
		t.Fatal("expected an error for a non-Agent object")
	}
	if !apierrors.IsBadRequest(err) {
		t.Errorf("error = %v, want a BadRequest", err)
	}
}

// A serverless agent is the one case where minReplicas: 0 is legal, so the
// always-on floor must not be applied to it.
func TestApplyDefaultsLeavesServerlessAtZero(t *testing.T) {
	s := &AgentSpec{Mode: ModeServerless, MinReplicas: 0}
	applyDefaults(s)

	if s.Mode != ModeServerless {
		t.Errorf("Mode = %q, want serverless", s.Mode)
	}
	if s.MinReplicas != 0 {
		t.Errorf("MinReplicas = %d, want 0 — the always-on floor must not apply to serverless", s.MinReplicas)
	}
}

// validateSpec accumulates every violation rather than bailing on the
// first, so an operator fixing a bad manifest sees the whole list at once.
func TestValidateSpecReportsAllViolations(t *testing.T) {
	a := &Agent{}
	a.Name = "broken"
	a.Spec = AgentSpec{
		Image:       "",           // required
		Mode:        ModeAlwaysOn, // with MinReplicas 0 -> violation
		MinReplicas: 0,
		MaxReplicas: 0,
		Concurrency: 0,     // must be >= 1
		Port:        70000, // out of range
	}

	err := validateSpec(a)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{
		"image is required",
		"must be >= 1 when mode is always-on",
		"must be 1..65535",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error is missing %q:\n%s", want, msg)
		}
	}

	// It should be a structured Invalid error so the API server renders
	// per-field messages, not an opaque string.
	if !apierrors.IsInvalid(err) {
		t.Errorf("error = %v, want a StatusReasonInvalid", err)
	}
	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("error type = %T, want *apierrors.StatusError", err)
	}
	causes := statusErr.ErrStatus.Details.Causes
	if len(causes) < 4 {
		t.Errorf("causes = %d, want at least 4 (image, minReplicas, concurrency, port)", len(causes))
	}
	if statusErr.ErrStatus.Details.Kind != "Agent" {
		t.Errorf("Details.Kind = %q, want Agent", statusErr.ErrStatus.Details.Kind)
	}
}
