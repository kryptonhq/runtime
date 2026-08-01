/*
Copyright 2026 Krypton Authors.
*/

package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"k8s.io/apimachinery/pkg/types"

	kryptonv1alpha1 "github.com/kryptonhq/runtime/api/v1alpha1"
)

//go:embed schema.sql
var schemaSQL string

// Postgres is a Store backed by Postgres via pgx.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres opens a connection pool and applies the embedded schema.
// Closing the returned Store closes the pool.
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Upsert(ctx context.Context, a *kryptonv1alpha1.Agent) error {
	specJSON, err := json.Marshal(a.Spec)
	if err != nil {
		return fmt.Errorf("marshal spec: %w", err)
	}
	statusJSON, err := json.Marshal(a.Status)
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}
	// Conflict on (namespace, name), not on uid. That pair is the row's
	// real identity — Get, Delete and List all key on it, and uid is only
	// ever carried, never looked up.
	//
	// Conflicting on uid instead breaks delete-and-recreate: Kubernetes
	// assigns a fresh UID, so the INSERT misses the uid conflict, hits the
	// (namespace, name) UNIQUE index, and errors out permanently for that
	// agent. That happens whenever the control plane isn't running to
	// observe the delete — e.g. a restart spanning a `kubectl delete` and
	// `kubectl apply`, or a GitOps prune-and-recreate.
	const q = `
		INSERT INTO agents (uid, namespace, name, spec, status, observed_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (namespace, name) DO UPDATE SET
			uid         = EXCLUDED.uid,
			spec        = EXCLUDED.spec,
			status      = EXCLUDED.status,
			observed_at = EXCLUDED.observed_at
	`
	if _, err := p.pool.Exec(ctx, q, string(a.UID), a.Namespace, a.Name, specJSON, statusJSON); err != nil {
		return fmt.Errorf("upsert agent: %w", err)
	}
	return nil
}

func (p *Postgres) Delete(ctx context.Context, key types.NamespacedName) error {
	const q = `DELETE FROM agents WHERE namespace = $1 AND name = $2`
	if _, err := p.pool.Exec(ctx, q, key.Namespace, key.Name); err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}

func (p *Postgres) Get(ctx context.Context, key types.NamespacedName) (*Record, error) {
	const q = `
		SELECT uid, namespace, name, spec, status
		FROM agents
		WHERE namespace = $1 AND name = $2
	`
	row := p.pool.QueryRow(ctx, q, key.Namespace, key.Name)
	rec, err := scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (p *Postgres) List(ctx context.Context, namespace string) ([]Record, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if namespace == "" {
		rows, err = p.pool.Query(ctx, `SELECT uid, namespace, name, spec, status FROM agents ORDER BY namespace, name`)
	} else {
		rows, err = p.pool.Query(ctx, `SELECT uid, namespace, name, spec, status FROM agents WHERE namespace = $1 ORDER BY name`, namespace)
	}
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	out := []Record{}
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

func (p *Postgres) Close() error {
	p.pool.Close()
	return nil
}

// scanRecord works against either pgx.Row or pgx.Rows since both expose Scan.
type scanner interface{ Scan(dest ...any) error }

func scanRecord(s scanner) (*Record, error) {
	var (
		rec        Record
		specJSON   []byte
		statusJSON []byte
	)
	if err := s.Scan(&rec.UID, &rec.Namespace, &rec.Name, &specJSON, &statusJSON); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(specJSON, &rec.Spec); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}
	if err := json.Unmarshal(statusJSON, &rec.Status); err != nil {
		return nil, fmt.Errorf("unmarshal status: %w", err)
	}
	return &rec, nil
}
