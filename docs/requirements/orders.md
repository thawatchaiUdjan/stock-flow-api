# Orders

Status: Confirmed (2026-11-07, agent under ADR 0005); amended by [business ADR 0013](../business-adr/0013-soft-delete.md), [business ADR 0014](../business-adr/0014-customers-and-the-storefront.md) and [business ADR 0015](../business-adr/0015-pay-online-in-test-mode.md)

## Purpose

An Order records a sale to one customer. Staff create and complete Orders through the day, and Customers place Online orders on the Storefront. Creating one takes its stock at once, so the stock shown is always what is still free to sell.

## Rules

- **R1** An Order has a number, an optional customer name of up to 120 characters, an optional note of up to 200, one or more Order lines, a total, a status, who created it (a User for an In-shop order, the Customer for an Online order, R21) and the time it was created (business ADR 0014).
- **R2** Order numbers count up from 1 in the order Orders are created, and are never reused.
- **R3** An Order line has a Product, a whole quantity of at least 1, and the Product's price when the Order was created, which never changes after. A Product is on one line of an Order at most.
- **R4** A line's amount is its quantity times its price, and the Order's total is the sum of its lines' amounts. There is no tax, discount or delivery charge.
- **R5** Every Role creates In-shop orders (Create Order, Users & access R11). Customers place Online orders on the Storefront (Storefront R10–R12).
- **R6** Only Active Products can be added to an Order (Products R12).
- **R7** Creating an Order takes each line's quantity from its Product's stock at once, as one Stock movement of the kind Order per line (ADR 0007).
- **R8** An Order with a line asking for more than its Product's stock is refused whole, saying for each such line how many are in stock. Nothing is taken.
- **R9** Orders created at the same moment for the same Product never together take more than its stock (Stock movements R7).
- **R10** An Order is Pending, Completed or Cancelled, and starts Pending.
- **R11** Any Role completes a Pending Order, once the customer has paid and has the goods; a Pay online Order only once Paid (R24). The Order records when it was completed and by whom.
- **R12** Only the Owner and Admins cancel a Pending Order in StockFlow; a Customer cancels their own Pending Online order (Storefront R16), and the system cancels an Unpaid one (R25). Cancelling gives each line's quantity back to its Product's stock, as one Stock movement of the kind Cancellation per line, even if the Product has been Archived since. The Order records when it was cancelled and by whom, or that the system did. A Paid Order is refunded first (R26).
- **R13** Completed and Cancelled are final.
- **R14** An Order's lines, customer and note cannot change after it is created. A mistake is fixed by cancelling the Order and creating a new one (ADR 0007).
- **R15** The Orders List shows each Order's number, when it was created, its customer, its number of lines, its total, its status and whether it is In-shop or Online, newest first.
- **R16** The Orders List can be searched by Order number or customer name, and filtered by status and by In-shop or Online.
- **R17** Opening an Order from the Orders List shows its lines, its total, who created it and, once it is no longer Pending, who completed or cancelled it and when, and for an Online order what R23 lists. It follows the Orders List's access (Users & access R11).
- **R18** Every Role sees every Order.
- **R19** While an Order is being created, each Product offered shows its stock, and the Order's total updates as lines change.
- **R20** Only the Owner and Admins delete an Order, and only a Completed or Cancelled one; a Pending Order is cancelled first. A Deleted Order is not in the Orders List, cannot be opened, and cannot be restored. Its Stock movements stay and its number is never reused (R2). Deleting does not change any Product's stock, and cancelling a Pending Order still gives stock back to a Product that was Archived and then Deleted (R12, business ADR 0013).
- **R21** An Order is an In-shop order, created by a User in StockFlow, or an Online order, placed by a Customer on the Storefront. Both share Order numbers (R2) and every rule here unless one says otherwise (business ADR 0014).
- **R22** An Online order also has its Customer, Pickup or Delivery, a Delivery address for Delivery (Storefront R7), Pay online or Pay on receipt, and for Pay online a payment state. Its customer name is the Customer's name when it was placed. None of these changes after placing (R14).
- **R23** Opening an Online order in StockFlow shows the Customer's name, email and phone number, Pickup or Delivery with the Delivery address, and the payment way and state.
- **R24** A Pay online Order is Unpaid until the payment provider confirms the payment, then Paid. It cannot be Completed while Unpaid (business ADR 0015).
- **R25** A Pay online Order still Unpaid 30 minutes after it was placed is Cancelled by the system, and its stock goes back as R12 says.
- **R26** Cancelling a Paid Order refunds its total in full and makes it Refunded. If the refund fails, the Order stays Pending and Paid, and the cancelling person is told to try again.
- **R27** An In-shop order and a Pay on receipt Order have no payment state; R11 completes them once the customer has paid and has the goods.

## Out of scope

- Customers as records for In-shop orders: there the customer is a name on the Order only. Online orders have a Customer (business ADR 0014).
- Payments for In-shop orders, partial payments, invoices and receipts. Online orders are paid online in test mode or on receipt (business ADR 0015).
- Tax, discounts and delivery charges.
- Returns after an Order is Completed.
- Editing an Order, and completing part of one.

## Open questions

None. Every rule here was decided by the agent under ADR 0005.
