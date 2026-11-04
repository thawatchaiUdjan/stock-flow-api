# 0006: Transaction pooler for the app, session pooler for migrations

Status: Proposed (Accepted once the first deploy confirms the poolers below)
Date: 2026-11-04

## Context

Every Vercel instance runs its own copy of the API (ADR 0008), so connections come from many short-lived processes. Supabase offers three ways in: the direct connection (IPv6 only on Free), the session pooler (port 5432) and the transaction pooler (port 6543). A transaction pooler hands each transaction to any server connection, so a statement prepared on one connection is missing on the next.

## Decision

- **The app** connects with `POSTGRES_URL`, injected by the Vercel integration and expected to be the transaction pooler (port 6543). `store.Open` sets pgx's `DefaultQueryExecMode` to `QueryExecModeSimpleProtocol`, so no statement is ever prepared on the server. The pool is small (`MaxConns` 4) with a short `MaxConnIdleTime` (30 s), so frozen instances don't hold pooler slots.
- **Migrations** connect with `MIGRATION_DATABASE_URL`, the session pooler (port 5432, IPv4). goose holds a session-level lock, which a transaction pooler can't keep, and GitHub-hosted runners have no IPv6, which rules out the Free direct connection.

## Pooler check

To be recorded from the first deploy, by host and port, before this ADR is accepted:

| Variable | Host | Port | Pooler |
| --- | --- | --- | --- |
| `POSTGRES_URL` | to be checked | 6543 expected | transaction expected |
| `POSTGRES_URL_NON_POOLING` | to be checked | to be checked | to be checked |

If `POSTGRES_URL` turns out to be the session pooler, the app keeps working (simple protocol is valid there too), but the many-instance argument above fails: change course in a new ADR.

## Consequences

- A slow test (`internal/store`) fails if a query leaves a prepared statement on the server, so the mode can't be dropped by accident.
- Simple protocol sends parameters as text. pgx handles the encoding; sqlc's output is unaffected.
- `MIGRATION_DATABASE_URL` is set by hand: locally in `.env`, and as a GitHub Actions secret. It doesn't come from Vercel.

## Alternatives considered

- Statement caching (`QueryExecModeCacheDescribe`/`CacheStatement`) behind the transaction pooler: fails as soon as a transaction lands on another server connection.
- The direct connection for migrations: IPv6 only on Free, and the runners have no IPv6.
