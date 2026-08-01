#!/usr/bin/env bash
# Boots a throwaway Postgres for the store integration tests and waits for
# it to accept connections. Idempotent; safe to re-run.
#
# Skips entirely when KRYPTON_TEST_POSTGRES_DSN already points somewhere
# (CI uses a service container instead of Docker-in-Docker).
#
# Tear down with: docker compose -f hack/docker-compose.postgres.yml down -v
set -euo pipefail

COMPOSE_FILE="$(dirname "$0")/docker-compose.postgres.yml"
DSN=${KRYPTON_TEST_POSTGRES_DSN:-}

if [[ -n "$DSN" ]]; then
  echo "▸ KRYPTON_TEST_POSTGRES_DSN is already set; not starting a container"
  exit 0
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "✖ docker not found — set KRYPTON_TEST_POSTGRES_DSN to an existing database instead" >&2
  exit 1
fi

echo "▸ starting postgres"
docker compose -f "$COMPOSE_FILE" up -d

echo "▸ waiting for postgres to accept connections"
for i in $(seq 1 60); do
  if docker compose -f "$COMPOSE_FILE" exec -T postgres \
       pg_isready -U krypton -d krypton >/dev/null 2>&1; then
    echo "✓ postgres ready after ${i}s"
    exit 0
  fi
  sleep 1
done

echo "✖ postgres did not become ready within 60s" >&2
docker compose -f "$COMPOSE_FILE" logs postgres >&2
exit 1
