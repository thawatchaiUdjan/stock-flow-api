# Stock movements

Status: Confirmed (2026-11-07, agent under ADR 0005)

## Purpose

Every change to a Product's stock is a Stock movement, so the stock shown always has its history behind it. Everyone in the Shop receives deliveries; the Owner and Admins correct the count when it is wrong.

## Rules

- **R1** A Product's stock is the sum of its Stock movements (ADR 0006).
- **R2** A Stock movement has a kind, a Product, a change in stock, the stock after it, the time, the User who made it and an optional note of up to 200 characters.
- **R3** There are four kinds: Receive and Adjustment, made by a User on the Stock Movements page, and Order and Cancellation, made by Orders (Orders R7, R12).
- **R4** A Receive adds a whole number of units, at least 1, to one Product's stock. Every Role can Receive.
- **R5** An Adjustment changes one Product's stock up or down by a whole number of units other than 0, and needs a note saying why. Only the Owner and Admins make Adjustments.
- **R6** No Stock movement takes a Product's stock below 0. One that would is refused, saying how many are in stock.
- **R7** Stock movements for one Product made at the same moment each apply to the stock the others left: none is lost, and together they never take the stock below 0.
- **R8** A Stock movement cannot be edited or deleted. A mistake is corrected by an Adjustment.
- **R9** Receive and Adjustment are not offered for an Archived Product (Products R12).
- **R10** The Stock Movements page lists Stock movements newest first, and can be filtered by Product, by kind and by a range of the Shop's days (Dashboard & reports R1).
- **R11** Each entry shows the time, the Product and its SKU, the kind, the change in stock, the stock after, the User and the note. One made by an Order links to that Order.
- **R12** Every Role sees every Stock movement.

## Out of scope

- Purchase prices, suppliers and purchase orders.
- More than one stock location, and transfers between them.
- Counting all stock at once (a stocktake); each correction is one Adjustment.
- Batches, serial numbers and expiry dates.

## Open questions

None. Every rule here was decided by the agent under ADR 0005.
