//go:build integration

/*
Copyright 2026 Krypton Authors.
*/

package store

import (
	"context"
	"os"
	"testing"
)

// TestPostgresStoreContract runs the shared Store contract against a real
// Postgres. Skipped by default; enable with `-tags integration` and point
// KRYPTON_TEST_POSTGRES_DSN at a database the test may write to.
//
//	hack/postgres-up.sh     # boots a throwaway postgres in docker
//	go test -tags integration ./internal/controlplane/store/...
func TestPostgresStoreContract(t *testing.T) {
	dsn := os.Getenv("KRYPTON_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KRYPTON_TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	pg, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	// Clean slate for repeatable runs.
	if _, err := pg.pool.Exec(ctx, `TRUNCATE agents`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	testStore(t, pg)
}
