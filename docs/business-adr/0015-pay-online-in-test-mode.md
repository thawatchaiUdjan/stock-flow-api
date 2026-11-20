# 0015: Online orders are paid online in test mode or on receipt

Status: Accepted
Date: 2026-11-20
Affects: api, web
Authorised by: Requirements owner (thawatchaiUdjan), `/grill-with-docs` session of 2026-11-20, as in business ADR 0014. The owner asked whether a payment provider can be tried for free, and otherwise to skip payment and let Staff confirm the Order. Merge not authorised.

## Context

Orders, *Out of scope*, excludes payments: a Completed Order simply means the customer paid and has the goods. The owner wants to practise online payment if a payment provider offers free testing. Stripe's test mode is free, supports Thailand and PromptPay, and moves no money ([technical ADR 0025](../adr/0025-payments-stripe-checkout-test-mode.md)).

## Decision

- **Two ways to pay an Online order.** *Pay online* on the payment provider's own page, by card or PromptPay; or *Pay on receipt*, paying the Shop at Pickup or Delivery. Pay online is offered only while the Shop has the provider set up. In-shop orders have no payment state and work as today.
- **Test mode only.** No real money moves, and the Storefront says so wherever Pay online is offered.
- **A Pay online Order starts Unpaid** and becomes Paid when the provider confirms the payment. It cannot be Completed while Unpaid.
- **Unpaid Orders give their stock back.** One still Unpaid 30 minutes after it was placed is Cancelled by the system, and its stock goes back.
- **Cancelling a Paid Order refunds it in full** through the provider, and the Order becomes Refunded. If the refund fails, the Order stays Pending and Paid, and the person cancelling is told to try again.
- **The Shop never sees card details.** They are entered only on the provider's page.

## Consequences

- Remove payments from Orders' *Out of scope*, keeping partial payments, invoices and receipts out. Add Orders rules for payment state, expiry and refunds.
- api: needs the provider's test keys from the owner, and a webhook the provider calls; without the keys only Pay on receipt is offered.
- Sales still count only Completed Orders, so a Paid Order not yet handed over is not Sales.
- Moving to real payments later needs live keys, the provider's business checks, and a decision on fees and failed refunds; it is a new decision, not a switch.

## Alternatives considered

- No online payment, Staff confirm only: the owner's fallback if no free test mode existed.
- Omise (Opn Payments) test mode: also free and Thai, but PromptPay must first be enabled by its support team.
- Keeping an Unpaid Order Pending until Staff cancel it: abandoned payments would hold stock indefinitely.
- Refunding outside the system: leaves the Order's payment state wrong.
