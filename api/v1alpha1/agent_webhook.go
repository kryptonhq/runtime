/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-krypton-ai-v1alpha1-agent,mutating=true,failurePolicy=fail,sideEffects=None,groups=krypton.ai,resources=agents,verbs=create;update,versions=v1alpha1,name=mutate-agent.krypton.ai,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-krypton-ai-v1alpha1-agent,mutating=false,failurePolicy=fail,sideEffects=None,groups=krypton.ai,resources=agents,verbs=create;update,versions=v1alpha1,name=validate-agent.krypton.ai,admissionReviewVersions=v1

// SetupWebhookWithManager registers defaulter + validator with the manager.
func (a *Agent) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(a).
		WithDefaulter(&AgentDefaulter{}).
		WithValidator(&AgentValidator{}).
		Complete()
}

// AgentDefaulter applies default values to an Agent.
type AgentDefaulter struct{}

var _ webhook.CustomDefaulter = &AgentDefaulter{}

// Default fills in optional fields. The OpenAPI defaults handle the simple
// cases; this hook covers anything not expressible as a static default.
func (d *AgentDefaulter) Default(_ context.Context, obj runtime.Object) error {
	a, ok := obj.(*Agent)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected *Agent, got %T", obj))
	}
	applyDefaults(&a.Spec)
	return nil
}

// applyDefaults centralizes default application so unit tests can exercise it
// without spinning up a webhook server.
func applyDefaults(s *AgentSpec) {
	if s.Mode == "" {
		s.Mode = ModeAlwaysOn
	}
	// always-on must have at least one replica; bump the floor if the
	// caller left it unset.
	if s.Mode == ModeAlwaysOn && s.MinReplicas == 0 {
		s.MinReplicas = 1
	}
	if s.Protocol == "" {
		s.Protocol = ProtocolA2A
	}
	if s.Port == 0 {
		s.Port = 8080
	}
	if s.InvocationPath == "" {
		s.InvocationPath = "/"
	}
	if s.Concurrency == 0 {
		s.Concurrency = 8
	}
	if s.MaxReplicas == 0 {
		s.MaxReplicas = 10
	}
	if s.ScaleToZeroAfter.Duration == 0 {
		s.ScaleToZeroAfter = metav1.Duration{Duration: 300 * time.Second}
	}
	if s.Timeout.Duration == 0 {
		s.Timeout = metav1.Duration{Duration: 60 * time.Second}
	}
	if s.StartupTimeout.Duration == 0 {
		s.StartupTimeout = metav1.Duration{Duration: 30 * time.Second}
	}
}

// AgentValidator enforces invariants beyond OpenAPI validation.
type AgentValidator struct{}

var _ webhook.CustomValidator = &AgentValidator{}

func (v *AgentValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	a, ok := obj.(*Agent)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected *Agent, got %T", obj))
	}
	return nil, validateSpec(a)
}

func (v *AgentValidator) ValidateUpdate(_ context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	a, ok := newObj.(*Agent)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected *Agent, got %T", newObj))
	}
	return nil, validateSpec(a)
}

func (v *AgentValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateSpec(a *Agent) error {
	var errs field.ErrorList
	spec := a.Spec
	specPath := field.NewPath("spec")

	if spec.Image == "" {
		errs = append(errs, field.Required(specPath.Child("image"), "image is required"))
	}
	if spec.Mode == ModeAlwaysOn && spec.MinReplicas < 1 {
		errs = append(errs, field.Invalid(
			specPath.Child("minReplicas"), spec.MinReplicas,
			"must be >= 1 when mode is always-on",
		))
	}
	if spec.Concurrency < 1 {
		errs = append(errs, field.Invalid(
			specPath.Child("concurrency"), spec.Concurrency,
			"must be >= 1",
		))
	}
	if spec.MaxReplicas < spec.MinReplicas {
		errs = append(errs, field.Invalid(
			specPath.Child("maxReplicas"), spec.MaxReplicas,
			"must be >= minReplicas",
		))
	}
	if spec.Port < 1 || spec.Port > 65535 {
		errs = append(errs, field.Invalid(
			specPath.Child("port"), spec.Port,
			"must be 1..65535",
		))
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: GroupVersion.Group, Kind: "Agent"},
		a.Name,
		errs,
	)
}
