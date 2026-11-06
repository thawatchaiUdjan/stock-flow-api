# 0018: Three Roles, in place of ADR 0015's two

Status: Accepted
Date: 2026-11-06

## Context

ADR 0015 says "Two roles: owner and staff." `stock-flow-context` ADR 0001 replaced that with three, once Users & access showed people who run the Shop beside the Owner but must not manage Users. An accepted ADR is never edited, so this one carries the change.

## Decision

This supersedes the roles line of ADR 0015, and only that line: there are three Roles, Owner, Admin and Staff (`users-and-access` R8). Authorization is still checked in Go, not with Supabase Row Level Security, and the rest of ADR 0015 stands.

A Role is a `text` column with a `CHECK`, holding `owner`, `admin` or `staff`, which are also the values the contract publishes. The one Owner (R9) is a partial unique index, so the database refuses a second one whatever the code does.

## Consequences

- `stock-flow-web` reads three Roles from the contract (`get-current-user`).
- Adding a Role is a migration and a contract change.

## Alternatives considered

- A Postgres `enum` for the Role: nothing gained over a `CHECK` at this size, and sqlc would add a Go type the handlers then convert to and from a string.
