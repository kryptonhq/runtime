/*
Copyright 2026 Krypton Authors.
*/

package v1alpha1

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestApplyDefaults(t *testing.T) {
	s := &AgentSpec{}
	applyDefaults(s)

	if s.Mode != ModeAlwaysOn {
		t.Errorf("Mode = %q, want %q", s.Mode, ModeAlwaysOn)
	}
	if s.MinReplicas != 1 {
		t.Errorf("MinReplicas = %d, want 1 (always-on floor)", s.MinReplicas)
	}
	if s.Protocol != ProtocolA2A {
		t.Errorf("Protocol = %q, want %q", s.Protocol, ProtocolA2A)
	}
	if s.Port != 8080 {
		t.Errorf("Port = %d, want 8080", s.Port)
	}
	if s.InvocationPath != "/" {
		t.Errorf("InvocationPath = %q, want /", s.InvocationPath)
	}
	if s.Concurrency != 8 {
		t.Errorf("Concurrency = %d, want 8", s.Concurrency)
	}
	if s.MaxReplicas != 10 {
		t.Errorf("MaxReplicas = %d, want 10", s.MaxReplicas)
	}
	if s.ScaleToZeroAfter.Duration != 300*time.Second {
		t.Errorf("ScaleToZeroAfter = %v, want 300s", s.ScaleToZeroAfter.Duration)
	}
	if s.Timeout.Duration != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", s.Timeout.Duration)
	}
	if s.StartupTimeout.Duration != 30*time.Second {
		t.Errorf("StartupTimeout = %v, want 30s", s.StartupTimeout.Duration)
	}
}

func TestApplyDefaultsPreservesExplicit(t *testing.T) {
	s := &AgentSpec{
		Mode:        ModeAlwaysOn,
		Protocol:    ProtocolMCP,
		Port:        9000,
		Concurrency: 64,
		MaxReplicas: 25,
	}
	applyDefaults(s)
	if s.Mode != ModeAlwaysOn {
		t.Errorf("Mode overwritten: %q", s.Mode)
	}
	if s.Protocol != ProtocolMCP {
		t.Errorf("Protocol overwritten: %q", s.Protocol)
	}
	if s.Port != 9000 {
		t.Errorf("Port overwritten: %d", s.Port)
	}
	if s.Concurrency != 64 {
		t.Errorf("Concurrency overwritten: %d", s.Concurrency)
	}
	if s.MaxReplicas != 25 {
		t.Errorf("MaxReplicas overwritten: %d", s.MaxReplicas)
	}
}

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Agent)
		wantError string // substring; "" means must pass
	}{
		{
			name:   "valid serverless",
			mutate: func(_ *Agent) {},
		},
		{
			name: "missing image",
			mutate: func(a *Agent) {
				a.Spec.Image = ""
			},
			wantError: "image is required",
		},
		{
			name: "always-on with zero minReplicas",
			mutate: func(a *Agent) {
				a.Spec.Mode = ModeAlwaysOn
				a.Spec.MinReplicas = 0
			},
			wantError: "must be >= 1 when mode is always-on",
		},
		{
			name: "concurrency zero",
			mutate: func(a *Agent) {
				a.Spec.Concurrency = 0
			},
			wantError: "must be >= 1",
		},
		{
			name: "max < min",
			mutate: func(a *Agent) {
				a.Spec.MinReplicas = 5
				a.Spec.MaxReplicas = 2
			},
			wantError: "must be >= minReplicas",
		},
		{
			name: "port out of range",
			mutate: func(a *Agent) {
				a.Spec.Port = 70000
			},
			wantError: "must be 1..65535",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newValidAgent()
			tc.mutate(a)
			err := validateSpec(a)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantError)
			}
		})
	}
}

func TestDefaulterAndValidatorPlugin(t *testing.T) {
	// Lightweight smoke test: the CustomDefaulter / CustomValidator interfaces
	// are satisfied (compile-time checks in agent_webhook.go) and the wrapped
	// methods route to the underlying helpers.
	ctx := context.Background()
	a := newValidAgent()
	a.Spec.Mode = ""
	a.Spec.Port = 0

	if err := (&AgentDefaulter{}).Default(ctx, a); err != nil {
		t.Fatalf("Default returned error: %v", err)
	}
	if a.Spec.Mode != ModeAlwaysOn || a.Spec.Port != 8080 {
		t.Fatalf("defaults not applied via Default(): %+v", a.Spec)
	}

	if _, err := (&AgentValidator{}).ValidateCreate(ctx, a); err != nil {
		t.Fatalf("ValidateCreate on a valid agent returned error: %v", err)
	}

	a.Spec.Image = ""
	if _, err := (&AgentValidator{}).ValidateCreate(ctx, a); err == nil {
		t.Fatal("ValidateCreate should reject an agent with empty image")
	}
}

func newValidAgent() *Agent {
	a := &Agent{}
	a.Name = "travel-agent"
	a.Namespace = "agents"
	a.Spec = AgentSpec{
		Image: "ghcr.io/org/travel-agent:latest",
	}
	applyDefaults(&a.Spec)
	return a
}
