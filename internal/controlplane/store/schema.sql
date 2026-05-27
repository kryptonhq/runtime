-- Krypton control plane registry schema.
--
-- This is intentionally a single file applied at startup with IF NOT EXISTS
-- everywhere. When the schema gains breaking changes (likely with M6
-- invocations and M8 metrics tables), we'll graduate to a real migration
-- tool (goose / golang-migrate). For now: idempotent bootstrap.

CREATE TABLE IF NOT EXISTS agents (
    uid          TEXT PRIMARY KEY,
    namespace    TEXT NOT NULL,
    name         TEXT NOT NULL,
    spec         JSONB NOT NULL,
    status       JSONB NOT NULL,
    observed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (namespace, name)
);

CREATE INDEX IF NOT EXISTS agents_namespace_idx ON agents (namespace);
CREATE INDEX IF NOT EXISTS agents_observed_at_idx ON agents (observed_at DESC);
