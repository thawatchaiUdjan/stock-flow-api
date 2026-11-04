# 0003: pgx v5 and sqlc for database access

Status: Accepted
Date: 2026-11-04

## Context

The database is PostgreSQL (ADR 0005). The rules that matter, such as creating an order and deducting its stock in one transaction under constraints, are easiest to read and reason about in plain SQL.

## Decision

Queries are plain SQL in `db/queries/`. sqlc generates type-safe Go for them into `internal/store` (package `store`), against the schema the goose migrations build (`sqlc.yaml` points at `db/migrations`). The driver is pgx v5 with `pgxpool`; sqlc targets `pgx/v5` directly, not `database/sql`.

## Consequences

- Generated code is committed. `go generate ./...` regenerates it, and CI fails if the committed output differs (ADR 0017).
- `*store.Queries` works on a pool, a connection or a transaction (`store.New(tx)`), which is how multi-statement rules will commit together.
- Queries must run in pgx's simple-protocol mode (ADR 0006). sqlc's output doesn't depend on the mode.

## Alternatives considered

- GORM: an ORM hides the SQL that enforces the rules, and its query building is checked at run time, not at compile time.
