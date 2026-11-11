# Products

Status: Confirmed (2026-11-07, agent under ADR 0005)

## Purpose

The Owner and Admins keep the list of what the Shop sells. Everyone in the Shop sees it, with each Product's stock and whether it is running low.

## Rules

- **R1** A Product has a name, a SKU, a unit, a price, a Minimum stock, a status and its stock.
- **R2** The name is required, up to 120 characters, in any language.
- **R3** The SKU is required, up to 40 characters of letters, digits and hyphens. It belongs to one Product only, compared without regard to letter case, and can be changed.
- **R4** The unit is the word the Product is counted in, such as "pcs" or "box": required, up to 20 characters.
- **R5** The price is what one unit sells for, in Thai baht to the satang, from ฿0.00 to ฿9,999,999.99. Changing it never changes an Order already created (Orders R3).
- **R6** Stock is a whole number of units, never below 0. It changes only through Stock movements (ADR 0006); the Product Form never sets it.
- **R7** A new Product starts with a stock of 0.
- **R8** The Minimum stock is a whole number, 0 or more, 0 by default: the level at or below which the Product needs restocking.
- **R9** A Product is Out of stock when its stock is 0, Low when its stock is above 0 and at or below its Minimum stock, and In stock otherwise.
- **R10** Only the Owner and Admins add and edit Products (the Product Form, Users & access R11).
- **R11** A Product is Active or Archived, Active when added. Only the Owner and Admins archive and restore Products. A Product is never deleted, so that its Orders and Stock movements keep pointing at it.
- **R12** Nobody can receive or adjust stock for an Archived Product, or add it to an Order. Its stock stays as it is.
- **R13** The Product List shows each Product's name, SKU, unit, price, stock and stock state, sorted by name.
- **R14** The Product List can be searched by name or SKU, and filtered by stock state and by status. It shows Active Products unless asked for Archived ones.
- **R15** Every Role sees the same Product List, prices included. Staff are offered no action that changes a Product.
- **R16** Each Product in the Product List links to its Stock movements.

## Out of scope

- Product images. They need a storage bucket set up in Supabase (`stock-flow-api` ADR 0009), which only the requirements owner can do. A later feature.
- Categories, variants, barcodes and suppliers.
- A cost price, and so any margin or profit figure.
- Importing Products from a file.
- Guarding against two people editing one Product at once: the later save wins.

## Open questions

None. Every rule here was decided by the agent under ADR 0005.
