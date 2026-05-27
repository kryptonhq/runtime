/*
Copyright 2026 Krypton Authors.
*/

package scaler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
	"github.com/kryptonhq/runtime/internal/metrics"
)

// Scaler watches Agents, polls their in-flight counts, and writes
// desiredReplicas to status. It implements manager.Runnable so the
// controller-manager can host it alongside the reconciler.
type Scaler struct {
	Client   client.Client
	Probe    LoadProbe
	Decider  Decider
	Interval time.Duration

	mu          sync.Mutex
	lastScaleUp map[types.NamespacedName]time.Time
}

var _ manager.Runnable = (*Scaler)(nil)

// Start runs the scaler tick loop until ctx is cancelled.
func (s *Scaler) Start(ctx context.Context) error {
	if s.Interval == 0 {
		s.Interval = time.Second
	}
	if s.lastScaleUp == nil {
		s.lastScaleUp = map[types.NamespacedName]time.Time{}
	}
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	logger := log.FromContext(ctx).WithValues("component", "scaler")
	logger.Info("started", "interval", s.Interval)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			s.tick(ctx)
		}
	}
}

func (s *Scaler) tick(ctx context.Context) {
	var list kryptonv1alpha1.AgentList
	if err := s.Client.List(ctx, &list); err != nil {
		log.FromContext(ctx).Error(err, "list agents")
		return
	}
	for i := range list.Items {
		if err := s.reconcileOne(ctx, &list.Items[i]); err != nil {
			log.FromContext(ctx).Error(err, "reconcile agent", "agent", client.ObjectKeyFromObject(&list.Items[i]))
		}
	}
}

func (s *Scaler) reconcileOne(ctx context.Context, agent *kryptonv1alpha1.Agent) error {
	key := client.ObjectKeyFromObject(agent)
	inflight, err := s.Probe.AgentInflight(ctx, key)
	if err != nil {
		return fmt.Errorf("probe inflight: %w", err)
	}

	in := Input{
		Agent:       agent,
		Inflight:    inflight,
		LastScaleUp: s.getLastScaleUp(key),
	}
	desired := s.Decider.Decide(in)
	if desired == agent.Status.DesiredReplicas {
		metrics.ScalerDecisionsTotal.WithLabelValues(key.Name, key.Namespace, "noop").Inc()
		metrics.AgentReplicasDesired.WithLabelValues(key.Name, key.Namespace).Set(float64(desired))
		return nil
	}

	direction := "down"
	if desired > agent.Status.DesiredReplicas {
		direction = "up"
		s.recordScaleUp(key)
	}
	metrics.ScalerDecisionsTotal.WithLabelValues(key.Name, key.Namespace, direction).Inc()
	metrics.AgentReplicasDesired.WithLabelValues(key.Name, key.Namespace).Set(float64(desired))

	patch := client.MergeFrom(agent.DeepCopy())
	agent.Status.DesiredReplicas = desired
	return s.Client.Status().Patch(ctx, agent, patch)
}

func (s *Scaler) getLastScaleUp(key types.NamespacedName) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastScaleUp[key]
}

func (s *Scaler) recordScaleUp(key types.NamespacedName) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastScaleUp == nil {
		s.lastScaleUp = map[types.NamespacedName]time.Time{}
	}
	s.lastScaleUp[key] = s.now()
}

func (s *Scaler) now() time.Time {
	if s.Decider.Now != nil {
		return s.Decider.Now()
	}
	return time.Now()
}
