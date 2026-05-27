/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package controller hosts the controller-runtime reconcilers for Krypton
// CRDs. The AgentReconciler turns Agent CRs into Deployments + Services,
// owns lifecycle, and surfaces status. Scaling decisions land in a separate
// component (see M7); this reconciler just applies desiredReplicas.
package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	// FinalizerName blocks deletion until child resources drain.
	FinalizerName = "krypton.ai/cleanup"

	labelManagedBy = "app.kubernetes.io/managed-by"
	labelAgent     = "krypton.ai/agent"
	managedByValue = "krypton-runtime"

	// Sidecar wiring. The user container listens on spec.Port; the sidecar
	// listens on sidecarPort and reverse-proxies to the user container.
	// In-cluster Services target the sidecar port via the named port.
	sidecarContainerName = "krypton-proxy"
	sidecarPortName      = "proxy"
	sidecarPort          = int32(8888)

	// DefaultProxyImage is used when AgentReconciler.ProxyImage is empty.
	// The deploy/helm chart (M10) will set this explicitly per release.
	DefaultProxyImage = "ghcr.io/kryptonhq/krypton-proxy:latest"
)

// AgentReconciler reconciles Agent objects.
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// ProxyImage is the sidecar image to inject. Empty means DefaultProxyImage.
	ProxyImage string
}

func (r *AgentReconciler) proxyImage() string {
	if r.ProxyImage != "" {
		return r.ProxyImage
	}
	return DefaultProxyImage
}

// +kubebuilder:rbac:groups=krypton.ai,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=krypton.ai,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=krypton.ai,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives the Agent CR towards its desired state.
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("agent", req.NamespacedName)

	var agent kryptonv1alpha1.Agent
	if err := r.Get(ctx, req.NamespacedName, &agent); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get agent: %w", err)
	}

	// Handle deletion via finalizer.
	if !agent.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &agent)
	}
	if !controllerutil.ContainsFinalizer(&agent, FinalizerName) {
		controllerutil.AddFinalizer(&agent, FinalizerName)
		if err := r.Update(ctx, &agent); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.ensureServiceAccount(ctx, &agent); err != nil {
		return ctrl.Result{}, err
	}
	deploy, err := r.ensureDeployment(ctx, &agent)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureService(ctx, &agent); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, &agent, deploy); err != nil {
		return ctrl.Result{}, err
	}

	logger.V(1).Info("reconciled", "phase", agent.Status.Phase, "replicas", agent.Status.Replicas)
	return ctrl.Result{}, nil
}

func (r *AgentReconciler) reconcileDelete(ctx context.Context, agent *kryptonv1alpha1.Agent) (ctrl.Result, error) {
	// Future: drain in-flight invocations via Activator. For now, owner refs
	// guarantee child cleanup; we just drop the finalizer.
	if controllerutil.ContainsFinalizer(agent, FinalizerName) {
		controllerutil.RemoveFinalizer(agent, FinalizerName)
		if err := r.Update(ctx, agent); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *AgentReconciler) ensureServiceAccount(ctx context.Context, agent *kryptonv1alpha1.Agent) error {
	if agent.Spec.ServiceAccountName != "" {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: agent.Name, Namespace: agent.Namespace},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
			sa.Labels = childLabels(agent)
			return controllerutil.SetControllerReference(agent, sa, r.Scheme)
		})
		return err
	})
}

func (r *AgentReconciler) ensureDeployment(ctx context.Context, agent *kryptonv1alpha1.Agent) (*appsv1.Deployment, error) {
	replicas := desiredReplicas(agent)
	saName := agent.Spec.ServiceAccountName
	if saName == "" {
		saName = agent.Name
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: agent.Name, Namespace: agent.Namespace},
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
			deploy.Labels = childLabels(agent)
			deploy.Spec.Replicas = &replicas
			// Selector is immutable once set.
			if deploy.Spec.Selector == nil {
				deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: podSelector(agent)}
			}
			deploy.Spec.Template.Labels = podSelector(agent)
			deploy.Spec.Template.Spec.ServiceAccountName = saName
			deploy.Spec.Template.Spec.ImagePullSecrets = agent.Spec.ImagePullSecrets
			deploy.Spec.Template.Spec.Containers = []corev1.Container{
				{
					Name:            "agent",
					Image:           agent.Spec.Image,
					ImagePullPolicy: agent.Spec.ImagePullPolicy,
					Ports: []corev1.ContainerPort{{
						Name:          "http",
						ContainerPort: agent.Spec.Port,
						Protocol:      corev1.ProtocolTCP,
					}},
					Env:       agent.Spec.Env,
					EnvFrom:   agent.Spec.EnvFrom,
					Resources: agent.Spec.Resources,
				},
				r.proxyContainer(agent),
			}
			return controllerutil.SetControllerReference(agent, deploy, r.Scheme)
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("ensure deployment: %w", err)
	}
	return deploy, nil
}

func (r *AgentReconciler) ensureService(ctx context.Context, agent *kryptonv1alpha1.Agent) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: agent.Name, Namespace: agent.Namespace},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
			svc.Labels = childLabels(agent)
			svc.Spec.Selector = podSelector(agent)
			svc.Spec.Ports = []corev1.ServicePort{{
				Name:       "http",
				Port:       agent.Spec.Port,
				TargetPort: intstr.FromString(sidecarPortName),
				Protocol:   corev1.ProtocolTCP,
			}}
			return controllerutil.SetControllerReference(agent, svc, r.Scheme)
		})
		return err
	})
}

// proxyContainer renders the krypton-proxy sidecar container spec.
func (r *AgentReconciler) proxyContainer(agent *kryptonv1alpha1.Agent) corev1.Container {
	return corev1.Container{
		Name:            sidecarContainerName,
		Image:           r.proxyImage(),
		ImagePullPolicy: agent.Spec.ImagePullPolicy,
		Ports: []corev1.ContainerPort{{
			Name:          sidecarPortName,
			ContainerPort: sidecarPort,
			Protocol:      corev1.ProtocolTCP,
		}},
		Env: []corev1.EnvVar{
			{Name: "KRYPTON_AGENT_NAME", Value: agent.Name},
			{Name: "KRYPTON_AGENT_NAMESPACE", Value: agent.Namespace},
			{Name: "KRYPTON_LISTEN_ADDR", Value: fmt.Sprintf(":%d", sidecarPort)},
			{Name: "KRYPTON_UPSTREAM_URL", Value: fmt.Sprintf("http://127.0.0.1:%d", agent.Spec.Port)},
			{Name: "KRYPTON_CONCURRENCY", Value: fmt.Sprintf("%d", agent.Spec.Concurrency)},
			{Name: "KRYPTON_MODE", Value: string(agent.Spec.Mode)},
			{Name: "KRYPTON_IDLE_TIMEOUT", Value: agent.Spec.ScaleToZeroAfter.Duration.String()},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString(sidecarPortName),
				},
			},
			PeriodSeconds: 2,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString(sidecarPortName),
				},
			},
			PeriodSeconds: 10,
		},
	}
}

func (r *AgentReconciler) updateStatus(ctx context.Context, agent *kryptonv1alpha1.Agent, deploy *appsv1.Deployment) error {
	// MergeFrom captures the agent's state before mutation so the
	// resulting Patch only carries fields we modified — keeps us from
	// clobbering Status.DesiredReplicas / LastInvocationAt that the
	// scaler and gateway write concurrently.
	patch := client.MergeFrom(agent.DeepCopy())
	prev := agent.Status.DeepCopy()

	agent.Status.Replicas = deploy.Status.Replicas
	agent.Status.ReadyReplicas = deploy.Status.ReadyReplicas
	agent.Status.URL = fmt.Sprintf("http://%s.%s.svc%s", agent.Name, agent.Namespace, agent.Spec.InvocationPath)
	agent.Status.ObservedGeneration = agent.Generation

	switch {
	case deploy.Status.ReadyReplicas >= 1:
		agent.Status.Phase = kryptonv1alpha1.PhaseReady
		setCondition(&agent.Status.Conditions, kryptonv1alpha1.ConditionAvailable, metav1.ConditionTrue, "Ready", "agent has ready replicas")
	case desiredReplicas(agent) == 0:
		agent.Status.Phase = kryptonv1alpha1.PhaseReady
		setCondition(&agent.Status.Conditions, kryptonv1alpha1.ConditionAvailable, metav1.ConditionTrue, "ScaledToZero", "idle, scaled to zero")
	default:
		agent.Status.Phase = kryptonv1alpha1.PhasePending
		setCondition(&agent.Status.Conditions, kryptonv1alpha1.ConditionAvailable, metav1.ConditionFalse, "WaitingForPods", "no ready replicas yet")
	}

	if statusEqual(prev, &agent.Status) {
		return nil
	}
	if err := r.Status().Patch(ctx, agent, patch); err != nil {
		return fmt.Errorf("patch status: %w", err)
	}
	return nil
}

func childLabels(agent *kryptonv1alpha1.Agent) map[string]string {
	return map[string]string{
		labelManagedBy: managedByValue,
		labelAgent:     agent.Name,
	}
}

func podSelector(agent *kryptonv1alpha1.Agent) map[string]string {
	return map[string]string{labelAgent: agent.Name}
}

// desiredReplicas honors explicit status.desiredReplicas if set; otherwise
// falls back to minReplicas. The scaling decider (M7) will own this field.
func desiredReplicas(agent *kryptonv1alpha1.Agent) int32 {
	if agent.Status.DesiredReplicas > 0 {
		return agent.Status.DesiredReplicas
	}
	return agent.Spec.MinReplicas
}

func setCondition(conds *[]metav1.Condition, condType string, status metav1.ConditionStatus, reason, msg string) {
	now := metav1.Now()
	for i := range *conds {
		c := &(*conds)[i]
		if c.Type != condType {
			continue
		}
		if c.Status == status && c.Reason == reason {
			c.Message = msg
			return
		}
		c.Status = status
		c.Reason = reason
		c.Message = msg
		c.LastTransitionTime = now
		return
	}
	*conds = append(*conds, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: now,
	})
}

// statusEqual does a shallow comparison sufficient to suppress idempotent
// status writes. Excludes lastTransitionTime in conditions because that
// fluctuates on each Now() call.
func statusEqual(a, b *kryptonv1alpha1.AgentStatus) bool {
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

// SetupWithManager wires the reconciler into a controller-runtime manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kryptonv1alpha1.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Complete(r)
}
