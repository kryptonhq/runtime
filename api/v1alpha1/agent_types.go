/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RuntimeMode controls the agent's pod lifecycle.
// +kubebuilder:validation:Enum=serverless;always-on
type RuntimeMode string

const (
	ModeServerless RuntimeMode = "serverless"
	ModeAlwaysOn   RuntimeMode = "always-on"
)

// Protocol identifies the wire protocol the agent speaks.
// +kubebuilder:validation:Enum=a2a;mcp;http
type Protocol string

const (
	ProtocolA2A  Protocol = "a2a"
	ProtocolMCP  Protocol = "mcp"
	ProtocolHTTP Protocol = "http"
)

// AgentPhase is a high-level summary of where an Agent is in its lifecycle.
// +kubebuilder:validation:Enum=Pending;Ready;Scaling;Failed
type AgentPhase string

const (
	PhasePending AgentPhase = "Pending"
	PhaseReady   AgentPhase = "Ready"
	PhaseScaling AgentPhase = "Scaling"
	PhaseFailed  AgentPhase = "Failed"
)

// AgentSpec defines the desired state of an Agent.
type AgentSpec struct {
	// Image is the container image to run.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// ImagePullPolicy mirrors corev1.PullPolicy.
	// +kubebuilder:default=IfNotPresent
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets references secrets used to pull the image.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Runtime is informational ("python", "node", "go", "custom").
	// +optional
	Runtime string `json:"runtime,omitempty"`

	// Framework is informational ("langgraph", "crewai", ...).
	// +optional
	Framework string `json:"framework,omitempty"`

	// Protocol the agent speaks.
	// +kubebuilder:default=a2a
	// +optional
	Protocol Protocol `json:"protocol,omitempty"`

	// Mode picks the runtime model. `always-on` is the supported MVP
	// default; `serverless` (scale-to-zero) is implemented but paused —
	// the code path stays, you just opt in explicitly with
	// `mode: serverless` + `minReplicas: 0`.
	// +kubebuilder:default=always-on
	// +optional
	Mode RuntimeMode `json:"mode,omitempty"`

	// ScaleToZeroAfter is the idle window before a serverless agent is scaled
	// to zero. Ignored in always-on mode.
	// +kubebuilder:default="300s"
	// +optional
	ScaleToZeroAfter metav1.Duration `json:"scaleToZeroAfter,omitempty"`

	// MinReplicas is the floor for replica counts. Defaults to 1 to match
	// the always-on mode; explicitly set to 0 when running in serverless.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas caps replica counts.
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Concurrency is the in-flight request cap per pod, used by the scaling
	// decider and enforced by the sidecar.
	// +kubebuilder:default=8
	// +kubebuilder:validation:Minimum=1
	// +optional
	Concurrency int32 `json:"concurrency,omitempty"`

	// Port the user container listens on.
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// InvocationPath is the HTTP path the gateway forwards invocations to.
	// +kubebuilder:default="/"
	// +optional
	InvocationPath string `json:"invocationPath,omitempty"`

	// Resources mirrors a standard pod resource block.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Env passes environment variables to the user container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// EnvFrom sources environment variables from ConfigMaps/Secrets.
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// Timeout bounds a single invocation.
	// +kubebuilder:default="60s"
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// StartupTimeout bounds how long a pod has to become ready.
	// +kubebuilder:default="30s"
	// +optional
	StartupTimeout metav1.Duration `json:"startupTimeout,omitempty"`

	// ServiceAccountName overrides the auto-created service account.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// AgentStatus is the observed state of an Agent.
type AgentStatus struct {
	// Phase is a coarse summary of where the Agent is.
	// +optional
	Phase AgentPhase `json:"phase,omitempty"`

	// Replicas is the current replica count of the underlying Deployment.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of pods that report ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// DesiredReplicas is set by the scaling decider. The reconciler applies
	// it to the underlying Deployment.
	// +optional
	DesiredReplicas int32 `json:"desiredReplicas,omitempty"`

	// URL is the in-cluster invocation URL for this agent.
	// +optional
	URL string `json:"url,omitempty"`

	// LastInvocationAt is updated by the gateway/activator path.
	// +optional
	LastInvocationAt *metav1.Time `json:"lastInvocationAt,omitempty"`

	// ObservedGeneration matches metadata.generation when the status reflects
	// the latest reconciled spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions track the agent's state transitions.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// Condition types used in AgentStatus.Conditions.
const (
	ConditionAvailable   = "Available"
	ConditionProgressing = "Progressing"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=ag
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Agent is a deployable AI agent or MCP server.
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentList is a list of Agents.
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
