# 0010: API owns its record and workflow

Status: Accepted
Date: 2026-11-11
Affects: api
Authorised by: thawatchaiUdjan's owner request, captured in [API issue #40](https://github.com/thawatchaiUdjan/stock-flow-api/issues/40), to migrate workflow ownership while preserving product behaviour; branch and reviewable change authorised, merge explicitly excluded. Acceptance takes effect only when the record PR is explicitly authorised and merged.

## Context

API previously depended on stock-flow-context for requirements, glossary, business decisions, planning, ticket creation and coordination. That dependency prevents an independent checkout from carrying its own workflow. Business and technical ADR numbers already overlap.

## Decision

Import the relevant committed record once from context commit `4c81bd45ff27cca31bd911e5e429b46c273265d2`. Preserve rule IDs, decisions, confirmations, historical citations and unresolved questions. API owns subsequent requirements and decisions locally. Context becomes historical evidence, not a required checkout, approval service or coordinator.

Use `docs/business-adr/` for business decisions and retain `docs/adr/` for technical decisions. Existing `context ADR NNNN` citations resolve to the imported business number. This decision supersedes business ADRs 0005 and 0008 **for API workflow only**, including mode-selection, context coordination and automatic-merge instructions. It does not revoke their historical product confirmations or change product/security behaviour. Honour the owner's current task scope and delegation; new business decisions require owner answers or explicit delegation, and merging requires explicit authorisation.

Plan with `/grill-with-docs`, local requirements/decisions and a record PR; then publish a local spec with `/to-spec`, API tickets with `/to-tickets`, and build with `/implement`, test-first, required checks and independent `code-review`, followed by an implementation PR. Imported record confirmation remains valid; new record changes become authoritative when merged.

API and web each own their work. The owner authorises necessary task-related issue requests and replies directly between them without extra handoff approval. Link both issues, explain behaviour and contract needs, and let the receiving repo plan and implement. Requests do not authorise new business rules. Block only dependent work. Unrelated external messages still need authorisation.

API retains generated contract publication. Keep published contracts compatible or explicitly version breaks. Notify web directly of relevant published changes. Web adopts a specific API commit and tests integration against it; it does not follow API main automatically. Claim adoption only after web confirms its pin and integration result.

## Consequences

- A single API checkout can plan and implement from its own record. No sibling is edited by this migration.
- Imported UI-facing rules preserve cross-module meaning but do not assign UI work to API or prove web adoption.
- Requirements can diverge across repos unless direct requests and owner decisions make agreement explicit.
- Runtime code, schema, security controls and OpenAPI are unchanged. Existing fast/slow test and review rules remain.
- [Migration provenance and rollback](../migrations/context-ownership.md) records hashes, local mappings and remaining dependencies. This branch prepares the accepted record; it is not authoritative on main until the record PR is explicitly authorised and merged.

## Alternatives considered

- Keep context as coordinator: rejected by the owner's independent-ownership request.
- Copy business ADRs into technical `docs/adr/`: collides with existing numbers and breaks historical meaning.
- Reinterpret or simplify imported product rules: exceeds the behaviour-preserving authorisation.
