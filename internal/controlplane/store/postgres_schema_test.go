//go:build integration

/*
Copyright 2026 Krypton Authors.
*/

package store

import (
	"context"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("KRYPTON_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KRYPTON_TEST_POSTGRES_DSN not set")
	}
	return dsn
}

// schema.sql is applied on every NewPostgres call — i.e. on every process
// start, and once per replica. It must therefore be idempotent. A missing
// IF NOT EXISTS would crash the second control-plane pod to start, which is
// exactly the kind of failure that only shows up under a rolling restart.
func TestSchemaIsIdempotent(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	first, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("first NewPostgres: %v", err)
	}
	defer func() { _ = first.Close() }()

	// Applying the schema again over an existing one must not error.
	second, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("second NewPostgres (schema is not idempotent): %v", err)
	}
	defer func() { _ = second.Close() }()

	third, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("third NewPostgres: %v", err)
	}
	_ = third.Close()
}

// Re-applying the schema must not drop existing rows. If someone replaces
// an IF NOT EXISTS with a DROP/CREATE, the registry would be wiped on every
// restart — silently, since the store is a mirror and would just re-fill.
func TestSchemaReapplyPreservesData(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	pg, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	if _, err := pg.pool.Exec(ctx, `TRUNCATE agents`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	agent := newAgent("persistent", "agents", "uid-persistent")
	if err := pg.Upsert(ctx, agent); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	_ = pg.Close()

	// Simulate a restart: fresh pool, schema re-applied.
	pg2, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = pg2.Close() }()

	got, err := pg2.Get(ctx, types.NamespacedName{Namespace: "agents", Name: "persistent"})
	if err != nil {
		t.Fatalf("row did not survive schema re-apply: %v", err)
	}
	if got.UID != "uid-persistent" {
		t.Errorf("UID = %q, want uid-persistent", got.UID)
	}
}

// Delete-and-recreate (new UID, same namespace/name) is covered for both
// backends by the shared contract in memory_test.go's testStore. This file
// only holds assertions that are specific to the SQL backend.

// Spec and status are stored as JSONB. Round-trip a spec with nested and
// composite fields to prove nothing is lost in serialization — the Postgres
// analogue of the CRD round-trip test.
func TestPostgresJSONBRoundTrip(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	pg, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	defer func() { _ = pg.Close() }()
	if _, err := pg.pool.Exec(ctx, `TRUNCATE agents`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	agent := newAgent("rich", "agents", "uid-rich")
	agent.Spec.Protocol = kryptonv1alpha1.ProtocolMCP
	agent.Spec.Mode = kryptonv1alpha1.ModeServerless
	agent.Spec.Concurrency = 32
	agent.Spec.MaxReplicas = 9
	agent.Spec.InvocationPath = "/mcp"
	agent.Status.Phase = kryptonv1alpha1.PhaseScaling
	agent.Status.Replicas = 2
	agent.Status.ReadyReplicas = 1
	agent.Status.URL = "http://rich.agents.svc:8080/mcp"
	agent.Status.Conditions = []metav1.Condition{{
		Type:               kryptonv1alpha1.ConditionAvailable,
		Status:             metav1.ConditionFalse,
		Reason:             "ScalingUp",
		Message:            "waiting for 1 more replica",
		LastTransitionTime: metav1.Now(),
	}}

	if err := pg.Upsert(ctx, agent); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := pg.Get(ctx, types.NamespacedName{Namespace: "agents", Name: "rich"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"spec.protocol", got.Spec.Protocol, kryptonv1alpha1.ProtocolMCP},
		{"spec.mode", got.Spec.Mode, kryptonv1alpha1.ModeServerless},
		{"spec.concurrency", got.Spec.Concurrency, int32(32)},
		{"spec.maxReplicas", got.Spec.MaxReplicas, int32(9)},
		{"spec.invocationPath", got.Spec.InvocationPath, "/mcp"},
		{"status.phase", got.Status.Phase, kryptonv1alpha1.PhaseScaling},
		{"status.replicas", got.Status.Replicas, int32(2)},
		{"status.readyReplicas", got.Status.ReadyReplicas, int32(1)},
		{"status.url", got.Status.URL, "http://rich.agents.svc:8080/mcp"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}

	if len(got.Status.Conditions) != 1 {
		t.Fatalf("conditions = %d, want 1", len(got.Status.Conditions))
	}
	cond := got.Status.Conditions[0]
	if cond.Reason != "ScalingUp" || cond.Message != "waiting for 1 more replica" {
		t.Errorf("condition lost detail in JSONB: %+v", cond)
	}
}

// List must order deterministically, or paginated reads in offline tooling
// would return overlapping or skipped rows.
func TestListIsDeterministicallyOrdered(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	pg, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	defer func() { _ = pg.Close() }()
	if _, err := pg.pool.Exec(ctx, `TRUNCATE agents`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	for _, name := range []string{"zulu", "alpha", "mike", "bravo"} {
		if err := pg.Upsert(ctx, newAgent(name, "agents", "uid-"+name)); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}

	first, err := pg.List(ctx, "agents")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(first) != 4 {
		t.Fatalf("list = %d rows, want 4", len(first))
	}

	// Repeat reads must produce the same order.
	for i := 0; i < 3; i++ {
		again, err := pg.List(ctx, "agents")
		if err != nil {
			t.Fatalf("list %d: %v", i, err)
		}
		for j := range first {
			if again[j].Name != first[j].Name {
				t.Fatalf("List order is not stable: position %d was %q, now %q",
					j, first[j].Name, again[j].Name)
			}
		}
	}
}
