# 0006: Stock changes only through Stock movements, which are never edited

Status: Accepted
Date: 2026-11-07
Affects: api, web

## Context

A Product's stock is the figure the whole product is built around (`stock-flow-web`'s `docs/design.md`). If anyone could type a new stock figure into the Product Form, a wrong count would leave no trace of who changed it, when or why, and Orders, deliveries and corrections could silently overwrite each other.

## Decision

A Product's stock is the sum of its Stock movements. Nothing sets the stock directly: a delivery is a Receive, a correction is an Adjustment, and Orders make their own movements (ADR 0007). A Stock movement is never edited or deleted; a mistake is corrected by another Adjustment. No movement may take stock below 0, including movements made at the same moment.

Decided by the agent under ADR 0005.

## Consequences

- Every stock figure can be explained, line by line, from the Stock Movements page.
- Correcting a mistake takes two entries, the wrong one and its Adjustment, and both stay visible.
- api: the stock shown and the sum of movements must never disagree, and movements for one Product made at the same moment must be serialised so that none is lost and stock never goes below 0. The api may keep the current stock on the Product for speed, provided it is changed only together with the movement that changes it.
- web: the Product Form has no stock field; stock changes happen on the Stock Movements page.

## Alternatives considered

- An editable stock figure on the Product: no history, and simultaneous changes overwrite each other.
- Editable movements: the history could no longer be trusted, and every figure built on it would move under its readers.
