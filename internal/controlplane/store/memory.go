/*
Copyright 2026 Krypton Authors.
*/

package store

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/types"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

// Memory is an in-process Store. Safe for concurrent use.
type Memory struct {
	mu   sync.RWMutex
	rows map[types.NamespacedName]Record
}

// NewMemory builds an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{rows: make(map[types.NamespacedName]Record)}
}

func (m *Memory) Upsert(_ context.Context, a *kryptonv1alpha1.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[types.NamespacedName{Namespace: a.Namespace, Name: a.Name}] = Record{
		UID:       string(a.UID),
		Namespace: a.Namespace,
		Name:      a.Name,
		Spec:      *a.Spec.DeepCopy(),
		Status:    *a.Status.DeepCopy(),
	}
	return nil
}

func (m *Memory) Delete(_ context.Context, key types.NamespacedName) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, key)
	return nil
}

func (m *Memory) Get(_ context.Context, key types.NamespacedName) (*Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rows[key]
	if !ok {
		return nil, ErrNotFound
	}
	cp := r
	return &cp, nil
}

func (m *Memory) List(_ context.Context, namespace string) ([]Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Record, 0, len(m.rows))
	for _, r := range m.rows {
		if namespace != "" && r.Namespace != namespace {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (*Memory) Close() error { return nil }
