# 0019: Domain error codes in Problem Details' `type`

Status: Accepted
Date: 2026-11-06

## Context

ADR 0014 left open whether domain errors get their own codes, to settle before the first feature that returns one. Users & access is that feature: "not logged in" and "session ended" are both 401, and the web app must tell them apart (`users-and-access` R26). The requirements owner decided on 2026-11-06 that each domain error carries a stable code in Problem Details' `type`, and that the contract publishes the list `stock-flow-web` checks against.

## Decision

- A domain error's `type` is a bare, stable, kebab-case code, such as `session-ended`. RFC 9457 allows a relative URI reference there.
- A plain HTTP error, with nothing more to say than its status, has no `type`, which RFC 9457 reads as `about:blank`. That covers the plain 401, the 500s and the 503.
- Field errors don't get a code: they keep `errors[].location` (ADR 0014).
- **The published list is the `enum` of `ErrorModel.type` in `openapi.yaml`**, with each code's meaning in the property's description. `publishErrorCodes` in `internal/api` writes it from the `errorCodes` list, in place of Huma's generated `format: uri` property, so `stock-flow-web`'s generated client types `type` as exactly those values.
- The statuses an operation can return stay declared on it (ADR 0014).

## Consequences

- This settles ADR 0014's open question.
- A new domain error is a new constant and a new line in `errorCodes`. Renaming or removing a code is a breaking contract change.
- The list says which codes exist, not which operation returns which: the operation's description and its statuses say that.

## Alternatives considered

- Full URLs in `type` (`https://…/errors/session-ended`): what RFC 9457 suggests when a page exists to read, and none does. The requirements owner chose the bare code.
- A separate `code` extension member: a second field for what `type` is for, and Huma's `ErrorModel` would need replacing.
- A separate unreferenced `ErrorCode` schema: nothing would tie it to the errors, and the client would not type `type`.
