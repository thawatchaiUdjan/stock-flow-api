# Orders

Status: Confirmed (2026-11-07, agent under ADR 0005)

## Purpose

An Order records a sale to one customer. Staff create and complete Orders through the day. Creating one takes its stock at once, so the stock shown is always what is still free to sell.

## Rules

- **R1** An Order has a number, an optional customer name of up to 120 characters, an optional note of up to 200, one or more Order lines, a total, a status, the User who created it and the time it was created.
- **R2** Order numbers count up from 1 in the order Orders are created, and are never reused.
- **R3** An Order line has a Product, a whole quantity of at least 1, and the Product's price when the Order was created, which never changes after. A Product is on one line of an Order at most.
- **R4** A line's amount is its quantity times its price, and the Order's total is the sum of its lines' amounts. There is no tax, discount or delivery charge.
- **R5** Every Role creates Orders (Create Order, Users & access R11).
- **R6** Only Active Products can be added to an Order (Products R12).
- **R7** Creating an Order takes each line's quantity from its Product's stock at once, as one Stock movement of the kind Order per line (ADR 0007).
- **R8** An Order with a line asking for more than its Product's stock is refused whole, saying for each such line how many are in stock. Nothing is taken.
- **R9** Orders created at the same moment for the same Product never together take more than its stock (Stock movements R7).
- **R10** An Order is Pending, Completed or Cancelled, and starts Pending.
- **R11** Any Role completes a Pending Order, once the customer has paid and has the goods. The Order records when it was completed and by whom.
- **R12** Only the Owner and Admins cancel a Pending Order. Cancelling gives each line's quantity back to its Product's stock, as one Stock movement of the kind Cancellation per line, even if the Product has been Archived since. The Order records when it was cancelled and by whom.
- **R13** Completed and Cancelled are final.
- **R14** An Order's lines, customer and note cannot change after it is created. A mistake is fixed by cancelling the Order and creating a new one (ADR 0007).
- **R15** The Orders List shows each Order's number, when it was created, its customer, its number of lines, its total and its status, newest first.
- **R16** The Orders List can be searched by Order number or customer name, and filtered by status.
- **R17** Opening an Order from the Orders List shows its lines, its total, who created it and, once it is no longer Pending, who completed or cancelled it and when. It follows the Orders List's access (Users & access R11).
- **R18** Every Role sees every Order.
- **R19** While an Order is being created, each Product offered shows its stock, and the Order's total updates as lines change.

## Out of scope

- Customers as records: the customer is a name on the Order only.
- Payments, partial payments, invoices and receipts.
- Tax, discounts and delivery.
- Returns after an Order is Completed.
- Editing an Order, and completing part of one.

## Open questions

None. Every rule here was decided by the agent under ADR 0005.
