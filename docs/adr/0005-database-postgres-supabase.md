# 0005: PostgreSQL on Supabase Free, Singapore

Status: Accepted
Date: 2026-11-04

## Context

Creating an order and deducting its stock must commit together or not at all, under constraints such as stock never going negative. Hosting must cost nothing (ADR 0016).

## Decision

PostgreSQL, hosted on Supabase Free in Singapore (`ap-southeast-1`). The project `stock-flow-db` already exists: created through Vercel's Supabase integration and connected to the Vercel project `stock-flow-api`, Production environment only, with no variable prefix and the variables marked sensitive.

## Consequences

- Transactions and `CHECK` constraints enforce the stock rules in the database, under the Go code that also checks them.
- Supabase Free pauses a project after a week without activity: the daily keepalive calls `/health`, which queries the database (ADR 0010).
- Supabase is used as Postgres and Storage (ADR 0009) only: no Supabase Auth and no Row Level Security. Authorization lives in Go (ADR 0015).
- Tests run against Postgres 17, the major version Supabase runs (ADR 0011).

## Alternatives considered

- A document store or SQLite: neither gives the transactional, constrained writes above on free hosting next to Vercel.
