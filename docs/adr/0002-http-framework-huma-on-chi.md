# 0002: Huma v2 on chi v5

Status: Accepted
Date: 2026-11-04

## Context

`stock-flow-web` generates its client from this repo's contract, so the contract must describe exactly what the server does. A contract written by hand, or assembled from comments, drifts from the code.

## Decision

HTTP routing is chi v5; operations are registered with Huma v2 through its `humachi` adapter. Huma derives the OpenAPI 3.1 document from the Go request and response types, and validates requests against the same types. Routes live in `internal/api`; `api.New` returns both the `http.Handler` and the `huma.API` whose document is the contract.

## Consequences

- The contract can't drift: changing a type changes the document (ADR 0013).
- Huma's defaults also give RFC 9457 errors (ADR 0014) and serve `/openapi.yaml`, `/openapi.json` and `/docs` live.
- Plain `net/http` middleware works on the chi router; the `Cache-Control: no-store` middleware is one (ADR 0016).

## Alternatives considered

- Gin + swaggo: the spec comes from comments that nothing checks against the code, and swaggo emits Swagger 2.0 only.
- oapi-codegen: spec-first, so the contract is written by hand and the code generated from it. That's the reverse of what this repo wants: the contract generated from the code.
