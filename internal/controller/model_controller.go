/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	resourceapi "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

const (
	// labelModel scopes child resources back to their owning Model CR.
	labelModel = "krypton.ai/model"

	// DefaultLlamaCppImage is the upstream llama.cpp HTTP server image
	// used when ModelSpec.Image is empty. llama-server can pull GGUF
	// weights directly from Hugging Face via --hf-repo / --hf-file.
	DefaultLlamaCppImage = "ghcr.io/ggml-org/llama.cpp:server"

	// modelCacheVolume is an emptyDir mounted at the llama.cpp HF cache
	// path so re-pulls within a pod lifetime hit local disk. Survives
	// container restarts within the same pod; not across pod replacement.
	modelCacheVolume = "model-cache"
	modelCacheMount  = "/root/.cache/llama.cpp"
)

// ModelReconciler reconciles Model objects into a Deployment + Service that
// serves an OpenAI-compatible HTTP endpoint.
type ModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// LlamaCppImage overrides the default llama.cpp image. Empty means
	// DefaultLlamaCppImage.
	LlamaCppImage string
}

func (r *ModelReconciler) llamaCppImage() string {
	if r.LlamaCppImage != "" {
		return r.LlamaCppImage
	}
	return DefaultLlamaCppImage
}

// +kubebuilder:rbac:groups=krypton.ai,resources=models,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=krypton.ai,resources=models/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=krypton.ai,resources=models/finalizers,verbs=update

// Reconcile drives the Model CR towards its desired state.
func (r *ModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("model", req.NamespacedName)

	var model kryptonv1alpha1.Model
	if err := r.Get(ctx, req.NamespacedName, &model); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get model: %w", err)
	}

	if !model.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&model, FinalizerName) {
			controllerutil.RemoveFinalizer(&model, FinalizerName)
			if err := r.Update(ctx, &model); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}
	if !controllerutil.ContainsFinalizer(&model, FinalizerName) {
		controllerutil.AddFinalizer(&model, FinalizerName)
		if err := r.Update(ctx, &model); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.ensureModelServiceAccount(ctx, &model); err != nil {
		return ctrl.Result{}, err
	}
	deploy, err := r.ensureModelDeployment(ctx, &model)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureModelService(ctx, &model); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.updateModelStatus(ctx, &model, deploy); err != nil {
		return ctrl.Result{}, err
	}

	logger.V(1).Info("reconciled", "phase", model.Status.Phase, "replicas", model.Status.Replicas)
	return ctrl.Result{}, nil
}

func (r *ModelReconciler) ensureModelServiceAccount(ctx context.Context, m *kryptonv1alpha1.Model) error {
	if m.Spec.ServiceAccountName != "" {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: m.Name, Namespace: m.Namespace},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
			sa.Labels = modelChildLabels(m)
			return controllerutil.SetControllerReference(m, sa, r.Scheme)
		})
		return err
	})
}

func (r *ModelReconciler) ensureModelDeployment(ctx context.Context, m *kryptonv1alpha1.Model) (*appsv1.Deployment, error) {
	replicas := m.Spec.MinReplicas
	if replicas <= 0 {
		replicas = 1
	}
	saName := m.Spec.ServiceAccountName
	if saName == "" {
		saName = m.Name
	}

	image := m.Spec.Image
	if image == "" {
		image = r.llamaCppImage()
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: m.Name, Namespace: m.Namespace},
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
			deploy.Labels = modelChildLabels(m)
			deploy.Spec.Replicas = &replicas
			if deploy.Spec.Selector == nil {
				deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: modelPodSelector(m)}
			}
			deploy.Spec.Template.Labels = modelPodSelector(m)
			deploy.Spec.Template.Spec.ServiceAccountName = saName
			deploy.Spec.Template.Spec.ImagePullSecrets = m.Spec.ImagePullSecrets
			deploy.Spec.Template.Spec.Volumes = []corev1.Volume{{
				Name: modelCacheVolume,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						SizeLimit: ptrQuantity("20Gi"),
					},
				},
			}}
			deploy.Spec.Template.Spec.Containers = []corev1.Container{{
				Name:            "llama-server",
				Image:           image,
				ImagePullPolicy: m.Spec.ImagePullPolicy,
				Args:            llamaCppArgs(m),
				Ports: []corev1.ContainerPort{{
					Name:          "http",
					ContainerPort: m.Spec.Port,
					Protocol:      corev1.ProtocolTCP,
				}},
				Env:       m.Spec.Env,
				EnvFrom:   m.Spec.EnvFrom,
				Resources: m.Spec.Resources,
				VolumeMounts: []corev1.VolumeMount{{
					Name:      modelCacheVolume,
					MountPath: modelCacheMount,
				}},
				// llama-server exposes /health once weights are loaded.
				// Generous failureThreshold accommodates first-pull which
				// can take minutes on cold disks.
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/health",
							Port: intstr.FromString("http"),
						},
					},
					PeriodSeconds:    5,
					FailureThreshold: 240,
				},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/health",
							Port: intstr.FromString("http"),
						},
					},
					PeriodSeconds:    20,
					FailureThreshold: 6,
				},
			}}
			return controllerutil.SetControllerReference(m, deploy, r.Scheme)
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("ensure model deployment: %w", err)
	}
	return deploy, nil
}

func (r *ModelReconciler) ensureModelService(ctx context.Context, m *kryptonv1alpha1.Model) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: m.Name, Namespace: m.Namespace},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
			svc.Labels = modelChildLabels(m)
			svc.Spec.Selector = modelPodSelector(m)
			svc.Spec.Ports = []corev1.ServicePort{{
				Name:       "http",
				Port:       m.Spec.Port,
				TargetPort: intstr.FromString("http"),
				Protocol:   corev1.ProtocolTCP,
			}}
			return controllerutil.SetControllerReference(m, svc, r.Scheme)
		})
		return err
	})
}

func (r *ModelReconciler) updateModelStatus(ctx context.Context, m *kryptonv1alpha1.Model, deploy *appsv1.Deployment) error {
	patch := client.MergeFrom(m.DeepCopy())
	prev := m.Status.DeepCopy()

	m.Status.Replicas = deploy.Status.Replicas
	m.Status.ReadyReplicas = deploy.Status.ReadyReplicas
	m.Status.URL = fmt.Sprintf("http://%s.%s.svc:%d/v1", m.Name, m.Namespace, m.Spec.Port)
	m.Status.ObservedGeneration = m.Generation

	if deploy.Status.ReadyReplicas >= 1 {
		m.Status.Phase = kryptonv1alpha1.ModelPhaseReady
		setCondition(&m.Status.Conditions, kryptonv1alpha1.ConditionAvailable, metav1.ConditionTrue, "Ready", "model has ready replicas")
	} else {
		m.Status.Phase = kryptonv1alpha1.ModelPhasePending
		setCondition(&m.Status.Conditions, kryptonv1alpha1.ConditionAvailable, metav1.ConditionFalse, "WaitingForPods", "no ready replicas yet")
	}

	if modelStatusEqual(prev, &m.Status) {
		return nil
	}
	if err := r.Status().Patch(ctx, m, patch); err != nil {
		return fmt.Errorf("patch model status: %w", err)
	}
	return nil
}

func modelChildLabels(m *kryptonv1alpha1.Model) map[string]string {
	return map[string]string{
		labelManagedBy: managedByValue,
		labelModel:     m.Name,
	}
}

func modelPodSelector(m *kryptonv1alpha1.Model) map[string]string {
	return map[string]string{labelModel: m.Name}
}

// llamaCppArgs renders the argument list passed to llama-server. The
// controller owns transport flags (--host/--port) and the HF source flags;
// users append tuning knobs via spec.args.
func llamaCppArgs(m *kryptonv1alpha1.Model) []string {
	args := []string{
		"--host", "0.0.0.0",
		"--port", fmt.Sprintf("%d", m.Spec.Port),
		"--hf-repo", m.Spec.Source.HuggingFace,
		"--hf-file", m.Spec.Source.File,
		// Advertise this exact name on /v1/models from llama-server. We
		// override it again at the gateway, but matching here avoids
		// surprises when callers hit the pod directly.
		"--alias", m.Name,
	}
	return append(args, m.Spec.Args...)
}

func modelStatusEqual(a, b *kryptonv1alpha1.ModelStatus) bool {
	if a.Phase != b.Phase ||
		a.Replicas != b.Replicas ||
		a.ReadyReplicas != b.ReadyReplicas ||
		a.URL != b.URL ||
		a.ObservedGeneration != b.ObservedGeneration ||
		len(a.Conditions) != len(b.Conditions) {
		return false
	}
	for i := range a.Conditions {
		ac, bc := a.Conditions[i], b.Conditions[i]
		if ac.Type != bc.Type || ac.Status != bc.Status || ac.Reason != bc.Reason {
			return false
		}
	}
	return true
}

func ptrQuantity(s string) *resourceapi.Quantity {
	q := resourceapi.MustParse(s)
	return &q
}

// SetupWithManager wires the reconciler into a controller-runtime manager.
func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kryptonv1alpha1.Model{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Complete(r)
}
