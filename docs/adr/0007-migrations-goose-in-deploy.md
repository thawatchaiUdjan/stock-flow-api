# 0007: goose migrations, run by the deploy workflow

Status: Accepted
Date: 2026-11-04

## Context

Schema changes must reach the database before the code that needs them. On Vercel every instance runs `main()`, so migrating at process startup would run once per instance, possibly several at the same time.

## Decision

Migrations are goose v3 SQL files in `db/migrations/` (`NNNNN_slug.sql`, with `-- +goose Up` and `-- +goose Down`). `deploy.yml` runs `go tool goose -dir db/migrations postgres "$MIGRATION_DATABASE_URL" up` before `vercel deploy --prod` (ADR 0017). The API never migrates.

`00001_baseline.sql` is a no-op: goose refuses an empty migrations directory, and both the deploy and the test database run `goose up`. The first feature's migration is `00002`.

## Consequences

- A migration must be compatible with the code still running while it applies: add first, remove in a later deploy.
- A failed migration stops the deploy, so the old code keeps running against an unchanged schema (goose applies each file in a transaction).
- Tests build their template database from the same files (ADR 0011).

## Alternatives considered

- Migrating at startup: runs per instance, possibly concurrently.
- Migrations in Go code, or another tool (Atlas, golang-migrate): goose's plain SQL files are also what sqlc reads as the schema.
