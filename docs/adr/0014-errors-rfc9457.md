# 0014: RFC 9457 Problem Details for errors

Status: Accepted
Date: 2026-11-04

A shared decision, made the same way in `stock-flow-web`'s stack ticket.

## Decision

Every error response is RFC 9457 Problem Details (`application/problem+json`), Huma's default `ErrorModel`: `type`, `title`, `status`, `detail`, `instance`, and `errors[]`, each with a `location` naming the offending field (e.g. `body.quantity`) and a `message`. Handlers return `huma.ErrorNNN…` values. Huma writes request-validation failures (422) the same way.

## Consequences

- The web app can map a field error to its form field by `location`, with no per-endpoint parsing.
- Each operation lists the statuses it can return (`huma.Operation.Errors`), so they appear in `openapi.yaml`.
- `AGENTS.md` also asks for one machine-readable list of every error code. Problem Details as decided carries no error code beyond the HTTP status, so for now the per-operation statuses in `openapi.yaml` are that list. Whether domain errors need their own codes (for example in `type`) is open for the first feature that returns one.

## Alternatives considered

- A custom `{ "error": { "code", "message" } }` envelope: non-standard, and Huma would need overriding to produce it.
