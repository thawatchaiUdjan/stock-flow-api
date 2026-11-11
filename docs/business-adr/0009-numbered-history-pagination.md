# 0009: Numbered history pagination

Status: Accepted
Date: 2026-11-10
Affects: api, web

## Context

Stock movements and Login log previously used cursor paging under Navigation & lists R6. A footer styling repair retained that behavior. The requirements owner clarified that these histories must match the other lists' numbered footer, including the matching count and First, Previous, Next and Last actions. The earlier styling-only interpretation was insufficient.

## Decision

Amend Navigation & lists R6 to give both histories numbered pages and an exact count for all current filters, excluding page selection. Retrieve bounded pages and the count from the server; do not download the entire history or fabricate totals. Preserve the existing cursor API's compatibility during rollout. Roles, filters, Bangkok date behavior and newest-first order remain unchanged. This decision was made under the owner's continuing task-scoped Autopilot delegation under ADR 0008.

## Consequences

- api supplies bounded numbered pages and exact filtered counts while maintaining existing cursor clients during rollout. No data or schema-destructive change is required.
- web replaces user-facing cursor controls with the shared numbered list footer, preserving filters, page size and page across reloads, resetting to the first page on filter or size changes, and providing recovery from an out-of-range page.
- Navigation parity takes precedence over cursor traversal efficiency. Counts and deeper offsets can cost more, and entries may shift between pages as new records arrive; a stable snapshot is not promised.

## Alternatives considered

- Keep cursor controls with shared styling: efficient traversal, but does not provide the numbered navigation and count the owner requested.
- Download all history or estimate counts: cannot provide bounded retrieval and an exact count as histories grow.
