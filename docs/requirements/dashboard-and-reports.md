# Dashboard & reports

Status: Confirmed (2026-11-07, agent under ADR 0005)

## Purpose

The Owner and Admins see at a glance how sales are going and what needs restocking, and look back over any period. Staff never see these figures (Users & access R11).

## Rules

- **R1** The Shop's days run on Bangkok time (UTC+7). Every "today", date and range of dates in the system is a day in that time.
- **R2** Sales are the totals of Completed Orders, counted on the day each was completed. Pending and Cancelled Orders are not Sales.
- **R3** The Dashboard shows today's Sales, the number of Orders completed today, the number of Pending Orders, and the numbers of Active Products that are Low and that are Out of stock.
- **R4** The Dashboard shows a chart of Sales for each of the last 14 days, today included.
- **R5** The Dashboard lists up to 10 Active Products that are Out of stock or Low, Out of stock first, each linking to its Stock movements, and links to the Product List filtered to them.
- **R6** The Dashboard lists the 5 newest Orders, each linking to it.
- **R7** Reports show, for a range of up to 366 days (this month so far by default): total Sales, the number of Orders completed, the average Sales per completed Order, and Sales for each day.
- **R8** Reports list up to 20 Products with the highest Sales in the range, each with the quantity sold and its Sales.
- **R9** Every figure matches the Orders as they stand when the page is opened.

## Out of scope

- Exporting figures to a file.
- Comparing a range with an earlier one.
- Profit and margin: Products have no cost price (Products, *Out of scope*).
- Reports sent by email.

## Open questions

None. Every rule here was decided by the agent under ADR 0005.
