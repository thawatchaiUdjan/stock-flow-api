# 0010: /health queries the database

Status: Accepted
Date: 2026-11-04

## Context

A Supabase Free project pauses after a week without activity (ADR 0005), and a paused database makes the deployed demo fail.

## Decision

`GET /health` runs `SELECT 1` through the app's pool (`store.Queries.Ping`) and answers `200 {"status":"ok"}`, or `503` Problem Details with detail `database unreachable` when the query fails. `keepalive.yml` calls it daily (ADR 0017), so the call counts as database activity.

## Consequences

- `/health` reports the API *and* its database. A 503 means the API runs but can't reach Postgres. The cause is logged, not returned.
- The check goes through `POSTGRES_URL`, so it also proves the transaction pooler path in production (ADR 0006).

## Alternatives considered

- A liveness-only `/health` plus a separate keepalive query: two endpoints, where one does both jobs.
