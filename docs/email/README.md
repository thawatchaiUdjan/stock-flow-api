# Transactional email

API owns these templates and their Brevo HTTP adapter. The runtime sends the same HTML and plain text that the preview command renders. [Business ADR 0011](../business-adr/0011-email-first-time-setup-and-self-service-recovery.md) owns the product rules; [spec #46](https://github.com/thawatchaiUdjan/stock-flow-api/issues/46) owns the implementation acceptance.

## Design and previews

Follow the existing web design at commit `0d634468a78e0f137719255d2980b2bfcdf63942`: Forest `#3A6B4F`, Page `#F6F7F4`, Ink `#20261F`, white surfaces and the Figtree/Noto Sans Thai font stack. Email clients without those fonts use Arial/sans-serif; there is no remote font dependency. Layout uses presentation tables, inline styles, wrapping fallback links and a phone media query.

| Message | HTML | Plain text | Phone, 390 px | Desktop, 800 px |
| --- | --- | --- | --- | --- |
| First-time code | [Open](previews/first-time-code.html) | [Open](previews/first-time-code.txt) | [Image](previews/first-time-code-phone.png) | [Image](previews/first-time-code-desktop.png) |
| Reset link | [Open](previews/reset-link.html) | [Open](previews/reset-link.txt) | [Image](previews/reset-link-phone.png) | [Image](previews/reset-link-desktop.png) |
| Reset completion | [Open](previews/reset-complete.html) | [Open](previews/reset-complete.txt) | [Image](previews/reset-complete-phone.png) | [Image](previews/reset-complete-desktop.png) |

All examples use fictional addresses and invalid demonstration credentials. Regenerate HTML/text from the repository root with `go run ./cmd/email-preview`; `go generate ./...` regenerates them too. After template changes, inspect all three actual variants at phone and desktop widths and refresh screenshots. Screenshots here were captured in headless Edge with explicit 390/800 px viewport emulation; all six had matching layout/scroll widths and were visually inspected. They are browser previews, not a claim of testing in Outlook, Gmail or every mail client.

## Configuration

Keep all four email variables empty for legacy-only startup. Configure all four together on the API project to enable the new operations:

- `BREVO_API_KEY`: transactional-email API key from an activated Brevo account.
- `EMAIL_SENDER_ADDRESS`: a mailbox you can access and individually verify in Brevo. Display name is StockFlow. An owned sending domain is not assumed.
- `WEB_BASE_URL`: trusted HTTPS origin of the web application, for example `https://your-web-project.vercel.app`. It must have no credentials, query, fragment or path other than an optional trailing slash. This is not the API URL or an email sending domain.
- `EMAIL_CODE_SECRET`: at least 32 random bytes, shared by all API instances, stored only in deployment secrets. Generate securely, do not type a memorable password. Coordinated rotation means already sent codes must be requested again.

Never paste secrets into chat or commit them. Partial/invalid settings stop startup with a value-free error; completely absent settings preserve existing functionality and make new email operations return `503 email-auth-unavailable`.

## Brevo setup and live verification

1. Create/activate a free Brevo account and verify your accessible sender mailbox using its inbox code. This needs mailbox access, not the mailbox password or Gmail SMTP.
2. Confirm transactional sending is available, then create the API key and set the variables through deployment secret settings. No paid upgrade is authorised.
3. Without a custom authenticated domain, Brevo may rewrite the sender address to a provider domain such as `brevosend.com`. Sender name stays StockFlow. Check the actual From address and inbox/spam delivery during the controlled live verification.
4. Check the provider quota before a controlled send. The documented free plan currently allows 300 sends per day; provider acceptance/queueing does not guarantee that a code arrives before its ten-minute expiry. Never extend credential validity to accommodate a late email; request fresh instructions under the same limits.
5. After the API release is published and web routes are ready, verify all three messages in a real inbox and check reset fragment navigation. Do not claim live delivery from mocked adapter tests.

Provider references, checked 2026-11-11: [sender verification](https://help.brevo.com/hc/en-us/articles/208836149-Create-a-new-sender-From-name-and-From-email), [sender rewriting](https://help.brevo.com/hc/en-us/articles/16045394674066-Troubleshooting-Domain-authentication), [free plan](https://help.brevo.com/hc/en-us/articles/208589409-About-Brevo-s-pricing-plans), [transactional API](https://developers.brevo.com/reference/send-transac-email).

No real provider credentials, account eligibility, verified sender or inbox delivery were available during implementation. If activation fails, report that blocker to the owner; do not substitute a provider or paid plan silently.

## Compatibility and rollback

Contract version 0.2.0 adds versioned email operations while retaining legacy operations. [Technical ADR 0023](../adr/0023-versioned-email-password-operations.md) and [retirement ticket #49](https://github.com/thawatchaiUdjan/stock-flow-api/issues/49) define the boundary: no automatic cutover, and legacy Owner-reset controls remain temporarily for the pinned client. Web must adopt the new flows and confirm its exact API pin/integration result before retirement.

Schema migration 00010 adds RLS-enabled credential and rolling request-event tables. Run migrations only through the existing deployment workflow in production. For application rollback, retain the additive tables and roll back code; do not run the Down migration on a used system or restore consumed/invalidated links. After new passwords have been set, prefer a forward fix so Users retain their recovery path. [Technical ADR 0024](../adr/0024-protect-email-code-and-credential-state.md) describes code-secret protection, rotation and concurrency.
