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

// ModelRuntime selects the inference backend baked into the Model pod.
// Only llama.cpp is wired today; vllm / tgi are placeholders for future work.
// +kubebuilder:validation:Enum=llama.cpp
type ModelRuntime string

const (
	RuntimeLlamaCpp ModelRuntime = "llama.cpp"
)

// ModelPhase is a coarse summary of a Model's lifecycle, mirroring AgentPhase.
// +kubebuilder:validation:Enum=Pending;Ready;Failed
type ModelPhase string

const (
	ModelPhasePending ModelPhase = "Pending"
	ModelPhaseReady   ModelPhase = "Ready"
	ModelPhaseFailed  ModelPhase = "Failed"
)

// ModelSource points at the weights to load. Today we only support pulling
// GGUF files straight from a Hugging Face repo; the llama.cpp server image
// resolves and caches the file at startup via --hf-repo / --hf-file.
type ModelSource struct {
	// HuggingFace is the repo id, e.g. "Qwen/Qwen2.5-0.5B-Instruct-GGUF".
	// +kubebuilder:validation:MinLength=1
	HuggingFace string `json:"huggingface"`

	// File names the GGUF file within the repo, e.g.
	// "qwen2.5-0.5b-instruct-q4_k_m.gguf". Required for GGUF repos that
	// publish multiple quantizations.
	// +kubebuilder:validation:MinLength=1
	File string `json:"file"`
}

// ModelSpec defines the desired state of a Model.
type ModelSpec struct {
	// Source describes where to fetch the weights.
	Source ModelSource `json:"source"`

	// Runtime is the inference backend. Only "llama.cpp" is supported today.
	// +kubebuilder:default=llama.cpp
	// +optional
	Runtime ModelRuntime `json:"runtime,omitempty"`

	// Image overrides the container image used to serve the model. When
	// empty, the controller uses its built-in default for the chosen
	// Runtime (see DefaultLlamaCppImage in the controller package).
	// +optional
	Image string `json:"image,omitempty"`

	// ImagePullPolicy mirrors corev1.PullPolicy.
	// +kubebuilder:default=IfNotPresent
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets references secrets used to pull the runtime image.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Port the inference server listens on.
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// Args are appended to the runtime entrypoint after the controller's
	// generated flags (--host, --port, --hf-repo, --hf-file). Use for
	// tuning context size, threading, etc.
	// +optional
	Args []string `json:"args,omitempty"`

	// MinReplicas is the floor for replica counts. Models are always-on:
	// scale-to-zero isn't supported yet because cold-loading multi-GB
	// weights through llama.cpp's HF cache makes activation impractical.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas caps replica counts (used by future autoscaling work).
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Resources mirrors a standard pod resource block.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Env passes environment variables to the runtime container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// EnvFrom sources environment variables from ConfigMaps/Secrets.
	// Used for HUGGINGFACE_HUB_TOKEN when pulling gated repos.
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// StartupTimeout bounds how long a pod has to become ready. Model
	// pulls can be slow; the default is generous to accommodate first-pull.
	// +kubebuilder:default="600s"
	// +optional
	StartupTimeout metav1.Duration `json:"startupTimeout,omitempty"`

	// ServiceAccountName overrides the auto-created service account.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// ModelStatus is the observed state of a Model.
type ModelStatus struct {
	// Phase is a coarse summary of where the Model is.
	// +optional
	Phase ModelPhase `json:"phase,omitempty"`

	// Replicas is the current replica count of the underlying Deployment.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of pods that report ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// URL is the in-cluster invocation URL for this model (OpenAI base).
	// +optional
	URL string `json:"url,omitempty"`

	// ObservedGeneration matches metadata.generation when status reflects
	// the latest reconciled spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions track the model's state transitions.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=mdl
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Runtime",type=string,JSONPath=`.spec.runtime`
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.source.huggingface`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Model is an inference-server backed LLM exposed over the OpenAI API.
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelSpec   `json:"spec,omitempty"`
	Status ModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelList is a list of Models.
type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Model{}, &ModelList{})
}
