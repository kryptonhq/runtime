/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package scaler computes desiredReplicas for each Agent and writes it to
// status. The M3 reconciler applies the value to the underlying
// Deployment. Keeping the decider separate from the reconciler isolates
// the policy from the mechanism and makes both easier to test.
package scaler

import (
	"time"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

// Decider holds tunables that aren't on the Agent spec. Implementations
// of Scaler embed one and call Decide for every agent on every tick.
type Decider struct {
	// StableWindow suppresses scale-down decisions within this duration of
	// the most recent scale-up. Prevents flapping under bursty load.
	StableWindow time.Duration

	// Now is overrideable for deterministic tests.
	Now func() time.Time
}

// Input is everything the decider needs to make one decision.
type Input struct {
	Agent       *kryptonv1alpha1.Agent
	Inflight    int
	LastScaleUp time.Time // zero if never scaled up in this process
}

// Decide returns the new desiredReplicas value for an agent. The caller
// patches status only when this differs from the current value.
//
// Formula:
//   - desired = clamp(ceil(inflight / concurrency), minReplicas, maxReplicas)
//   - serverless + inflight = 0 + idle past scaleToZeroAfter ⇒ 0
//   - within StableWindow of last scale-up, never scale down
func (d *Decider) Decide(in Input) int32 {
	spec := in.Agent.Spec
	now := d.now()

	// Concurrency-driven floor.
	desired := int32(0)
	if spec.Concurrency > 0 && in.Inflight > 0 {
		c := int(spec.Concurrency)
		desired = int32((in.Inflight + c - 1) / c)
	}

	idleAndCold := in.Inflight == 0 && d.isIdle(in.Agent, now)
	switch {
	case spec.Mode == kryptonv1alpha1.ModeAlwaysOn:
		// Always-on never scales below minReplicas (or 1 if min == 0).
		floor := spec.MinReplicas
		if floor < 1 {
			floor = 1
		}
		if desired < floor {
			desired = floor
		}
	case idleAndCold:
		desired = 0
	default:
		// Serverless agents stay warm with at least one replica while
		// inside their idle window — otherwise we'd flap to zero between
		// requests, fighting with the activator.
		if spec.Mode == kryptonv1alpha1.ModeServerless &&
			in.Agent.Status.LastInvocationAt != nil && desired < 1 {
			desired = 1
		}
		if desired < spec.MinReplicas {
			desired = spec.MinReplicas
		}
	}

	if spec.MaxReplicas > 0 && desired > spec.MaxReplicas {
		desired = spec.MaxReplicas
	}

	// Hysteresis: refuse to scale below the current desired within the
	// stable window of the last scale-up.
	cur := in.Agent.Status.DesiredReplicas
	if d.StableWindow > 0 && !in.LastScaleUp.IsZero() &&
		now.Sub(in.LastScaleUp) < d.StableWindow && desired < cur {
		return cur
	}
	return desired
}

func (d *Decider) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Decider) isIdle(a *kryptonv1alpha1.Agent, now time.Time) bool {
	if a.Spec.Mode != kryptonv1alpha1.ModeServerless {
		return false
	}
	// Never-invoked + zero replicas: stay scaled to zero. Once we've
	// scaled the agent up at least once, require an idle window before
	// going back to zero.
	if a.Status.LastInvocationAt == nil {
		return true
	}
	idle := a.Spec.ScaleToZeroAfter.Duration
	if idle == 0 {
		idle = 5 * time.Minute
	}
	return now.Sub(a.Status.LastInvocationAt.Time) >= idle
}
