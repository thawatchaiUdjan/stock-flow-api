# 0014: Customers order on the Storefront, apart from Users

Status: Accepted
Date: 2026-11-20
Affects: api, web
Authorised by: Requirements owner (thawatchaiUdjan), `/grill-with-docs` session of 2026-11-20. The owner asked for a Storefront where Customers log in and order, and for an audit log, and delegated the design: "design it and decide everything, then tell me; ask only what must be asked". The owner then answered the three questions asked: keep a Delivery address even though nothing is really delivered, take payments through a free test mode if one exists, let Customers both sign up and be created by the Shop with room for Google, Facebook and LINE sign-in later, and write the record and open a record PR. Merge not authorised.

## Context

Every Order today is created by a User in StockFlow, and the customer is only a name on it ([Orders](../requirements/orders.md), *Out of scope*). The owner wants customers to log in to a website of their own and place Orders there. The glossary defined a **User** as anyone who logs in, Users & access R4 forbids signing up, and business ADR 0001 fixes three Roles, so a customer cannot simply become a fourth kind of User.

## Decision

- **Customers are not Users.** A Customer has no Role, logs in only to the Storefront and never reaches StockFlow; a User's login never opens the Storefront. The same email may belong to a User and to a Customer. The three Roles and Users & access R4 stay as they are.
- **Two ways in.** Anyone signs up on the Storefront after confirming their email with a Verification code, and the Owner and Admins can create a Customer, who then sets their own password through the same code. Signing up with an email that already has a Customer says so, as Login's email step does (business ADR 0012).
- **Room for other sign-in methods.** A Customer is the person, identified by their confirmed email; a password is one way for them to sign in. Signing in with Google, Facebook or LINE is not built now, but nothing in these rules ties a Customer to having a password. LINE and Facebook may not share an email, so linking one later will have to ask for an email and confirm it.
- **An Online order is an Order.** It is numbered, takes stock when placed, counts as Sales when Completed and follows Orders R2–R14 like an In-shop order (business ADR 0007). It records the Customer instead of a User, Pickup or Delivery, a Delivery address for Delivery, and Pay online or Pay on receipt (business ADR 0015).
- **Nothing is really delivered.** This is a practice project: Delivery keeps a full Thai address, but no courier, delivery charge or tracking exists. Staff complete the Order once it has been handed over, as today.
- **Stock cannot be held hostage.** Because placing takes stock, a Customer has at most 3 Pending Online orders at once, each with at most 30 lines of at most 999 units. A Customer cancels their own Pending Online order and its stock goes back.
- **Prices are what the Customer saw.** Placing is refused, with the current prices, if any price changed after the Customer's checkout showed it.
- **The Shop manages Customers.** The Owner and Admins see the Customers list, create Customers, make them Inactive or Active again, and Delete Inactive ones as business ADR 0013 does for Users. Staff see a Customer's contact details on that Customer's Orders only.

## Consequences

- Add the Customers and Storefront modules; amend Orders R1, R12, R15, R16, R17, Users & access R11, R36, R38, Stock movements R2, R11 and Navigation & lists R4, R5 without renumbering. Add Storefront terms to the glossary and narrow **User** to people who work in the Shop.
- api: an Order and a Stock movement are made by a User, a Customer or, for a payment that never arrived, the system. Existing responses that assume a User are a contract change to plan in the spec, keeping the pinned web client working until it adopts.
- api: Customers have their own sessions, so a Customer's session can never pass a StockFlow Role check.
- web: the Storefront is a new set of public pages; StockFlow gains the Customers list and Online order details.
- A Customer deleting their own personal data (Thailand's PDPA) is not built, because a practice project holds no real Customers. It must be settled before real Customers sign up.

## Alternatives considered

- A fourth Role, Customer: breaks business ADR 0001 and Users & access R4, and puts strangers in the Users list and Login rules built for staff.
- Customers created only by the Shop: safer, but not a website anyone can order from; the owner wanted both.
- An Online order that takes no stock until the Shop accepts it: Customers could be told later that nothing is left, and a second status machine would sit beside business ADR 0007.
- Pickup only: simpler, but the owner wants to practise addresses.
- A delivery charge: changes Orders R4 and every total and Sales figure, with nothing really delivered.
