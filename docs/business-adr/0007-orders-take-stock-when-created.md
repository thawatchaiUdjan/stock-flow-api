# 0007: An Order takes its stock when created, and is never edited

Status: Accepted
Date: 2026-11-07
Affects: api, web

## Context

The Shop needs the stock it sees to be what it can still sell, while Staff work several Orders through the day. Stock could leave when an Order is created, or only when it is completed. And an Order with a mistake could be edited in place, or replaced.

## Decision

Creating an Order takes each line's quantity from stock at once, as Stock movements of the kind Order. An Order asking for more than is in stock is refused whole. Completing an Order changes no stock; cancelling a Pending Order gives the stock back, as movements of the kind Cancellation. An Order's lines, customer and note never change after it is created: a mistake is fixed by cancelling it and creating a new one. Only Completed Orders count as Sales (Dashboard & reports R2).

Decided by the agent under ADR 0005.

## Consequences

- Two Staff can never both sell the last unit: the second Order is refused at once, not at completion.
- Stock held by Pending Orders that are never completed stays taken until someone cancels them.
- There is no editing logic to keep in step with stock: every stock change of an Order is one of two moments, creation and cancellation.
- api: creating an Order, with all its movements, happens as one step that either fully applies or not at all, and is safe against simultaneous Orders for the same Product (ADR 0006).
- web: Create Order shows the stock of each Product as it is chosen, and the Orders List offers no Edit, only Complete and Cancel.

## Alternatives considered

- Taking stock at completion: two Pending Orders could both promise the last unit.
- Reserving stock separately from taking it: a second figure per Product to explain, more than a small Shop needs.
- Editable Orders: every edit would need its own stock movements, for little gain over cancel and recreate.
