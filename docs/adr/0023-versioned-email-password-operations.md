# 0023: Versioned email password operations during web adoption

Status: Accepted
Date: 2026-11-11
Affects: api, web
Authorised by: Owner-confirmed business ADR 0011, merged record PR #45 and delegated technical scope in API spec #46; implementation tickets #47 and #48. No merge or cutover authorisation.

## Context

The pinned web client consumes create-user responses containing a manually shared Setup link and an Owner-reset operation that immediately clears the password and sessions. Email creation exposes no credential to the Owner, and self-service recovery preserves access until completion. Reusing the old operations would change their published meanings.

## Decision

Publish API contract version 0.2.0 with additive `/v2/users`, `/v2/password-setup/*` and `/v2/password-reset/*` operations. Retain existing operations and response semantics during adoption. Login, session, User listing and editing stay shared; existing error codes remain in the published enum.

Legacy password changes and deactivation invalidate email credentials in the same transaction during coexistence. New password completion also invalidates legacy Setup links. No runtime flag silently converts old operations into the new flows.

Track retirement separately in [API ticket #49](https://github.com/thawatchaiUdjan/stock-flow-api/issues/49), blocked on confirmed web integration, exact adopted API pin and explicit cutover authorisation. The written handoff requested by the owner supplies the adoption instructions; it is not adoption evidence.

## Consequences

- The expansion release supports the pinned client while enabling new web implementation.
- Owner-triggered legacy resets remain temporarily reachable during compatibility; they are absent from the new operations and must retire at the deliberate cutover.
- Schema changes are additive. Roll back application code without dropping tables or reviving consumed credentials. Forward fixes are preferred after new flows have been used.
- Publish the generated contract before asking web to adopt. No sibling edits or web issue messages.

## Alternatives considered

- Repurpose the existing operations in place: breaks pinned-client semantics.
- Remove legacy routes immediately: prevents staged adoption and violates the confirmed compatibility requirement.
- Convert every API operation to a new version: unrelated work; only changed flows need new operations.
