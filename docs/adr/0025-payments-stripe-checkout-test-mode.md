# 0025: Pay online through Stripe Checkout in test mode

Status: Accepted
Date: 2026-11-20
Affects: api, web
Authorised by: Business ADR 0015 and the requirements owner's task-scoped delegation of 2026-11-20 to design the Storefront and its payments.

## Context

Business ADR 0015 adds Pay online, in test mode only, by card or PromptPay, with refunds on cancel and no card details reaching the Shop. The API runs as a Vercel Go function (ADR 0008) with no background worker.

## Decision

- Use **Stripe Checkout** (a hosted page) in **test mode**. Stripe supports businesses in Thailand and PromptPay in THB; in test mode its PromptPay QR opens a test page that approves or fails the payment. Card details stay on Stripe's page.
- Placing a Pay online Order creates a Checkout Session for its total, expiring after 30 minutes (Stripe's minimum), and returns its URL to the Storefront.
- A **webhook** verified with Stripe's signing secret marks the Order Paid on `checkout.session.completed` and cancels it on `checkout.session.expired`, so the 30-minute expiry needs no scheduler. Handling is idempotent: a repeated event changes nothing.
- Cancelling a Paid Order calls Stripe's refund API for the full amount first; a failed refund refuses the cancellation.
- The secret key and webhook signing secret are optional environment settings, as for email. Without them, Pay online is not offered.

## Consequences

- The owner creates a free Stripe account, which starts in test mode, and sets the keys and webhook URL in Vercel; agents cannot do this.
- Webhooks reach only the deployed API; local testing forwards events with the Stripe CLI.
- Adds the Stripe Go client as a dependency, pinned under ADR 0012.

## Alternatives considered

- Omise (Opn Payments): free test mode and Thai-first, but PromptPay is enabled by emailing support.
- An embedded card form: brings card handling, and its compliance scope, onto the Storefront.
- A scheduled job to expire Unpaid Orders: Vercel Hobby cron jobs run at most daily; Stripe's expiry event is on time.
