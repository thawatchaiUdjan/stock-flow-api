# 0013: Products, Users and Orders can be Deleted, and the record stays

Status: Accepted
Date: 2026-11-18
Affects: api, web
Authorised by: Requirements owner (thawatchaiUdjan), task brief of 2026-11-18 asking for soft-delete of Products, Orders and Users with web integration afterwards ([API issue #59](https://github.com/thawatchaiUdjan/stock-flow-api/issues/59)), and the owner's answers to the grilling questions in the same session: a separate Deleted layer, who may delete and when, no restore, SKU and email reusable, and a Deleted Order not counted as Sales. Merge not authorised.

## Context

Products R11 and Users R17 say a Product or a User is never deleted, so that Orders, Stock movements and the Login log keep pointing at them. Archived, Inactive and Cancelled hide things from daily use but never from the lists: a Shop with test data or a mistaken entry has no way to clear it. The owner wants removal that keeps the record. This replaces "never deleted" in Products R11 and Users R17.

## Decision

A Product, a User or an Order can be **Deleted**. The row stays in the database, marked with when and by whom; the Shop no longer sees it.

- **A separate layer.** Deleted is not a new Archived, Inactive or Cancelled, and those keep their meaning. A Product is Archived and then Deleted; a User is Inactive and then Deleted.
- **Who and when.**

  | | Who | Only when |
  | --- | --- | --- |
  | Product | Owner, Admin | Archived |
  | User | Owner | Inactive, and never the Owner |
  | Order | Owner, Admin | Completed or Cancelled; a Pending Order holds stock and is cancelled first |

- **Hidden, not erased.** A Deleted record is in no list, cannot be opened or changed, and takes part in nothing new: no stock received into it, no Order for it, no login by it.
- **No restore.** Nothing in the system brings a Deleted record back.
- **What keeps pointing at it.** Orders, Stock movements and the Login log keep showing a Deleted Product's or User's name and SKU or email. A Deleted Order's Stock movements stay (Stock movements R8), and stock does not change when an Order is Deleted.
- **Identifiers are freed.** A Deleted Product's SKU and a Deleted User's email can be used again, compared as before without regard to letter case.
- **Not Sales.** A Deleted Completed Order is not Sales: it is out of the Dashboard and Reports.

## Consequences

- Amend Products R3, R11, Users & access R3, R17 and Dashboard & reports R2 without renumbering; add Products R17, Users & access R54, Orders R20, Stock movements R13 and Dashboard & reports R10. Add **Deleted** to the glossary.
- The API adds three operations and three error codes, and hides Deleted records from existing responses. No existing field, status or code changes meaning, so the pinned web client keeps working until it adopts the operations.
- A Stock movement can name an Order that no longer opens; web must not link to it.
- Reusing a User's email gives the new User no link to the old one's history.
- A Deleted Product's id and an Order's number are never reused.
- A Deleted Product is always Archived, so Receive, Adjustment and Order refuse it as Archived (`product-archived`) rather than as unknown. A Pending Order that sells it can still be cancelled, and its stock goes back to the Deleted Product (orders R12).

## Alternatives considered

- Reusing Archived, Inactive or Cancelled as "deleted": they must stay visible and reversible; rejected by the owner.
- Reserving a Deleted Product's SKU and User's email: simpler, but a Shop could be told a SKU is taken by something it cannot see; rejected by the owner.
- Restoring Deleted records: not wanted for now.
- Deleting a Pending Order, or an Active Product or User: would need rules for the stock an Order holds and for Orders that sell the Product; not chosen.
- Counting a Deleted Order as Sales: figures would no longer match what the Orders List shows; rejected by the owner.
