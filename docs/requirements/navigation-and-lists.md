# Navigation & lists

Status: Amended by [ADR 0009](../business-adr/0009-numbered-history-pagination.md)

## Purpose

Every User can identify StockFlow, return to their landing page and move between the pages their Role allows. Lists remain manageable as records grow.

## Rules

- **R1** The application has a project icon beside the StockFlow name in the sidebar. Together they form one accessible link to the User's landing page (Users & access R12); the equivalent home link is available on a phone.
- **R2** Main navigation lists destinations, excluding New product and New order. Creating a Product or Order starts from its list. Removing these navigation items does not change access to the forms or their direct links (Users & access R11, R22).
- **R3** Authenticated pages have a top navigation bar indicating the current page and linking to its parent list when applicable. Main navigation highlights Products for Product forms and Orders for Order forms and details. Navigation respects the User's Role and remains usable on a phone and with a keyboard.
- **R4** The Product List, Orders List, Users list, Stock movements and Login log display one bounded page of records at a time, with visible paging controls even when there is only one page. Loading, errors and empty results have explicit states; an unavailable direction cannot be activated.
- **R5** Product List, Orders List, Users list, Stock movements and Login log offer a page size of 10, 25 or 50, defaulting to 25, and show the current page and matching count. Searches, filters, page size and page survive reloads; searches, filters and page size survive paging. Changing a search, filter or page size returns to the first page. A page beyond the available results offers a working way back to valid results.
- **R6** Stock movements and Login log use numbered pages with First, Previous, Next and Last controls. Their matching count is exact for all current filters across the full result, excluding the page selection. Records remain newest first. The system retrieves bounded server pages and the count without downloading the entire history or inventing totals.
- **R7** The Reports daily sales table is paginated at 25 rows per page; its chart and totals still describe the full selected date range. Its bounded Top products table and Dashboard summaries are not paginated.
- **R8** Stock movements and Login log offer a page size of 10, 25 or 50, defaulting to 25. The selected size survives paging and reloads. Changing the size or a filter returns to the first page; changing the size retains the filters.

## Out of scope

- Changing Role permissions, landing destinations, list sort order or business operations.
- Changing API contracts where existing responses suffice: Products and Orders already supply offset paging and totals; Users and Reports can page their complete responses in the web app. Stock movements and Login log require bounded numbered pages and exact totals, while their existing cursor API remains compatible during rollout.
- A new colour scheme. Icon artwork and navigation layout belong in the web repo's design direction.

## Open questions

None. The requirements owner requested the icon, home link, removal of the two create shortcuts, paginated lists and a navigation bar. R3's top-bar interpretation and R4-R7's detailed paging behaviour are agent decisions under ADR 0005. R8 was decided by the agent under the requirements owner's task-scoped Autopilot selection on 2026-11-10 under ADR 0008. The owner subsequently clarified that both history footers must match the numbered list footer; the agent decided the resulting R5, R6 and R8 amendments under that continuing delegation in ADR 0009.
