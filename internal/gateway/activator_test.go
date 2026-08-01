/*
Copyright 2026 Krypton Authors.
*/

package gateway

import (
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

func TestBufferDepthTracksSlots(t *testing.T) {
	a := &Activator{MaxBufferPerAgent: 2}
	key := types.NamespacedName{Namespace: "agents", Name: "travel"}

	// Nil map: reading a depth before any slot is taken must not panic.
	if got := a.BufferDepth(key); got != 0 {
		t.Fatalf("initial BufferDepth = %d, want 0", got)
	}

	if !a.acquireSlot(key) {
		t.Fatal("first acquireSlot failed")
	}
	if got := a.BufferDepth(key); got != 1 {
		t.Errorf("after 1 acquire, BufferDepth = %d, want 1", got)
	}

	if !a.acquireSlot(key) {
		t.Fatal("second acquireSlot failed")
	}
	if got := a.BufferDepth(key); got != 2 {
		t.Errorf("after 2 acquires, BufferDepth = %d, want 2", got)
	}

	// At the cap: further acquires are refused and the depth stays put.
	if a.acquireSlot(key) {
		t.Error("acquireSlot succeeded past MaxBufferPerAgent")
	}
	if got := a.BufferDepth(key); got != 2 {
		t.Errorf("after a refused acquire, BufferDepth = %d, want 2", got)
	}

	a.releaseSlot(key)
	if got := a.BufferDepth(key); got != 1 {
		t.Errorf("after release, BufferDepth = %d, want 1", got)
	}

	// Releasing more than was acquired must floor at zero, not go negative —
	// a negative gauge would corrupt the BufferDepth metric.
	a.releaseSlot(key)
	a.releaseSlot(key)
	if got := a.BufferDepth(key); got != 0 {
		t.Errorf("after over-release, BufferDepth = %d, want 0", got)
	}
}

func TestBufferDepthIsPerAgent(t *testing.T) {
	a := &Activator{MaxBufferPerAgent: 1}
	travel := types.NamespacedName{Namespace: "agents", Name: "travel"}
	billing := types.NamespacedName{Namespace: "agents", Name: "billing"}

	if !a.acquireSlot(travel) {
		t.Fatal("acquire travel failed")
	}
	// travel is at its cap, but that must not block a different agent.
	if a.acquireSlot(travel) {
		t.Error("travel exceeded its own cap")
	}
	if !a.acquireSlot(billing) {
		t.Error("billing was blocked by travel's full buffer")
	}

	if got := a.BufferDepth(travel); got != 1 {
		t.Errorf("travel depth = %d, want 1", got)
	}
	if got := a.BufferDepth(billing); got != 1 {
		t.Errorf("billing depth = %d, want 1", got)
	}
}

// The gateway calls acquire/release from many request goroutines at once;
// the depth must stay consistent under -race.
func TestBufferDepthConcurrentAcquireRelease(t *testing.T) {
	const goroutines = 50
	a := &Activator{MaxBufferPerAgent: goroutines}
	key := types.NamespacedName{Namespace: "agents", Name: "travel"}

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a.acquireSlot(key) {
				a.releaseSlot(key)
			}
		}()
	}
	wg.Wait()

	if got := a.BufferDepth(key); got != 0 {
		t.Errorf("BufferDepth after balanced acquire/release = %d, want 0", got)
	}
}
