# Storefront

Status: Confirmed (2026-11-20, requirements owner's task-scoped delegation in business ADR 0014 and business ADR 0015)

## Purpose

The Storefront is the Shop's public website. Anyone browses what the Shop sells; Customers put Products in a Cart, place Online orders, pay, and follow their Orders ([business ADR 0014](../business-adr/0014-customers-and-the-storefront.md)).

## Rules

- **R1** The Storefront is separate from StockFlow. A Customer's session reaches only the Storefront (Customers R1).
- **R2** Anyone browses the catalogue without logging in. Placing an Order, My orders and a Customer's profile need a Customer login; a visitor sent to log in returns to where they were.
- **R3** The catalogue shows Active Products only, sorted by name, one bounded page at a time, searched by name or SKU. Each shows its name, SKU, unit, price and availability.
- **R4** Availability is *In stock*, *Only N left* when the Product is Low (Products R9), or *Out of stock*. The Storefront shows no other stock figure, and an Out of stock Product cannot be added to the Cart.
- **R5** The Cart lives on the Customer's device and is not saved by the Shop. Each line is a Product and a whole quantity of at least 1. Opening the Cart or checkout shows each Product's current price, availability and the total.
- **R6** At checkout the Customer chooses Pickup or Delivery, and Pay online or Pay on receipt, and may add a note of up to 200 characters. Pay online is offered only while the Shop has payments set up (business ADR 0015).
- **R7** Delivery needs a Delivery address in Thailand: a recipient name of up to 120 characters, a phone number as Customers R4, an address line of up to 200 characters, a sub-district and a district of up to 80 characters each, one of Thailand's 77 provinces, and a 5-digit postcode.
- **R8** Checkout fills the Delivery address from the Customer's saved one, and the Customer may save what they enter as their saved Delivery address.
- **R9** There is no delivery charge, and nothing is really delivered: Staff complete the Order once it is handed over (Orders R4, R11).
- **R10** Placing an Order is refused whole, with each changed line's current price, if any price differs from what checkout showed. Nothing is taken.
- **R11** Placing an Order follows Orders R6, R8 and R9: Archived and Deleted Products are refused, and lines asking for more than the stock are refused with how many are left.
- **R12** A Customer has at most 3 Pending Online orders; an Online order has at most 30 lines and at most 999 units on a line. Placing beyond these is refused.
- **R13** After placing a Pay online Order, the Customer goes to the payment page, pays by card or PromptPay, and returns to the Order. Until it is Paid or 30 minutes have passed, the Order offers to go back to the payment page (Orders R25).
- **R14** Wherever Pay online appears, the Storefront says payments are in test mode and no real money moves.
- **R15** My orders lists the Customer's own Orders, newest first, with each one's number, when it was placed, its status, its payment state, its number of lines and its total. Opening one shows its lines, Pickup or Delivery and the Delivery address, the payment way and state, its note, and when it was placed, completed or cancelled. A Customer never sees another's Order, and a Deleted Order is not in My orders.
- **R16** A Customer cancels their own Pending Online order (Orders R12). A Paid one is refunded (Orders R26).
- **R17** The Customer is emailed when an Online order is placed, when it is Paid, when it is cancelled (saying whether by them, by the Shop or because payment did not arrive) and when it is Completed. A failed email changes nothing about the Order.
- **R18** The Storefront never shows Users, other Customers, Stock movements, Sales or any page of StockFlow.

## Out of scope

- Product images, descriptions, categories and variants (Products, *Out of scope*).
- Promotions, coupons, tax and delivery charges (Orders R4).
- Guest checkout without a Customer login.
- A Cart saved across devices.
- Order tracking, couriers and a status between Pending and Completed.
- Returns after Completed (Orders, *Out of scope*).
- Real payments (business ADR 0015).

## Open questions

None. Every rule here was decided by the agent under the requirements owner's task-scoped delegation of 2026-11-20 (business ADR 0014, business ADR 0015).
