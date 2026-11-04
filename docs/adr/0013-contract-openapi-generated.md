# 0013: REST/JSON, with the OpenAPI 3.1 contract generated from the code and committed

Status: Accepted
Date: 2026-11-04

A shared decision, made the same way in `stock-flow-web`'s stack ticket.

## Context

`stock-flow-web` generates its client from this repo's contract. The contract has to match the server exactly, and the web repo needs a stable file to read.

## Decision

- The API is REST/JSON. Huma generates its OpenAPI 3.1 document from the code (ADR 0002).
- `cmd/openapi` writes it to `openapi.yaml` at this repo's root, via `go generate ./...`. The file is committed, and `ci.yml` fails if it differs from what the code generates.
- `stock-flow-web` keeps a copy at `openapi/openapi.yaml`, refreshed by `bun run api:sync` from this repo's `main`, and generates its client from it with Orval. Its CI fails if regenerating changes the committed client. All three repos are public.

## Consequences

- A contract change shows up as a diff to `openapi.yaml` in the pull request, where the *Contract* section names what it breaks.
- The running API serves the same document at `/openapi.yaml` and `/openapi.json`, with docs at `/docs`.
- A contract change merges here before the web change that depends on it.

## Alternatives considered

- GraphQL or tRPC: more machinery than a small CRUD API needs, and tRPC requires a TypeScript server.
- Serving the document only at run time: the web repo couldn't refresh its copy without a deployed API.
