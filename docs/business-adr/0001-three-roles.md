# 0001: Three Roles: Owner, Admin and Staff

Status: Accepted
Date: 2026-11-06
Affects: api, web

## Context

The stack decisions assumed two roles, owner and staff (`stock-flow-api` ADR 0015, `stock-flow-web` ADR 0019, and the voice section of `stock-flow-web`'s `docs/design.md`). Defining Users & access showed a third level: people who run the Shop beside the Owner, but who must not manage Users.

## Decision

There are three Roles: Owner, Admin and Staff. Exactly one User is the Owner, created at installation. The Owner creates every other User as Admin or Staff. What each Role reaches is `users-and-access` R11.

## Consequences

- This replaces the "two roles" line of the stack decisions. Everything else in those ADRs stands.
- api: add a new ADR that supersedes the roles line of ADR 0015, and authorize all three Roles in Go.
- web: correct the role count in ADR 0019 and in `docs/design.md` ("Roles are Owner and Staff"), and show navigation per Role.

## Alternatives considered

- Two roles, with Admins as further Owners: the Owner could no longer be the only one who manages Users.
- Naming the top Role Super-admin: "Owner" is already the word in the record and says who it is.
