# Web handoff: first-time email setup and self-service password recovery

## Authority and delivery status

The owner confirmed the complete design on 2026-11-11 and selected Autopilot. The owner explicitly requested this written handoff for all web changes, rather than API agents editing web or posting web issue requests. Web owns its record, plan, implementation and PR. No merge or production cutover authority is implied.

- Confirmed product record: [API PR #45](https://github.com/thawatchaiUdjan/stock-flow-api/pull/45), merged at `691599bb0baa85fecb82d100e0a34c0c186bf807`.
- Requirements: [users-and-access R27–R34 and R41–R49](../requirements/users-and-access.md), [business ADR 0011](../business-adr/0011-email-first-time-setup-and-self-service-recovery.md), [glossary](../../GLOSSARY.md).
- API spec: [#46](https://github.com/thawatchaiUdjan/stock-flow-api/issues/46). Implementation: [#47](https://github.com/thawatchaiUdjan/stock-flow-api/issues/47) and [#48](https://github.com/thawatchaiUdjan/stock-flow-api/issues/48).
- API contract publication commit: `94d834a7280b87ad4c9314ec2d6fb5c3d44be649`, contract version **0.2.0**. [Generated OpenAPI at that exact commit](https://github.com/thawatchaiUdjan/stock-flow-api/blob/94d834a7280b87ad4c9314ec2d6fb5c3d44be649/openapi.yaml).
- This commit is for the additive API implementation PR. Publication of source/contract is not proof of merge, production deployment, configured email, or inbox delivery. Confirm API deployment readiness before switching live web traffic.
- Web's existing pin, observed read-only at web commit `0d634468a78e0f137719255d2980b2bfcdf63942`, is API commit `2384eabf44f0cebfd6f01feedd44a75d1b78178c`. Adoption of the new pin is **unconfirmed**.

## Adopt the contract

In the web repository, follow its own `docs/agents/api-contract.md`. Set `openapi/api-source.json` to this API repository and full publication commit above; do not use a moving branch. Run the existing commands `bun run api:sync`, `bun run api:generate` and `bun run api:check`. Commit the pin, adopted contract, generated client and frontend changes together. Do not hand-edit generated client files.

The existing production rewrite forwards `/api/:path*` to the API. Thus a new API route such as `/v2/password-reset/request` is reached by web as `/api/v2/password-reset/request`, using the existing fetcher and cookie handling. Keep the rewrite and existing session behavior; do not introduce a separate untrusted reset origin.

## New operations

All bodies are JSON. Exact schemas and automatic validation responses are in the pinned OpenAPI. Tokens/codes never belong in API paths or query parameters. All responses remain `Cache-Control: no-store`.

| API route, all POST | Operation ID | Input | Success |
| --- | --- | --- | --- |
| `/v2/users` | `create-email-user` | Existing `name`, `email`, Admin/Staff `role`; Owner session required | 201: `user` plus `emailDelivery` (`sent`, `failed`, `suppressed`); no Setup link or code |
| `/v2/password-setup/request` | `request-first-time-code` | `email` | 202: eligibility-neutral `message` |
| `/v2/password-setup/verify` | `verify-first-time-code` | `email`, six-digit `code` string, preserving leading zeroes | 200: `token`, `expiresAt`; token is a password-setup grant, not a session |
| `/v2/password-setup/password` | `set-email-password` | Verified grant `token`, `password` | 204, no login/session |
| `/v2/password-reset/request` | `request-password-reset` | `email` | 202: same eligibility-neutral `message` |
| `/v2/password-reset/password` | `reset-email-password` | Reset-link `token`, new `password` | 204, no login/session; all prior sessions ended |

The public request message is: `If this email is eligible, we've sent instructions.` Use the same success view for unknown, Inactive, limited and provider-send-failure cases. Do not infer eligibility, delivery or request type from 202. Infrastructure errors may still be generic 500; unconfigured email access is 503.

`emailDelivery: sent` means the provider accepted the email, not that it reached the inbox. On `failed` or `suppressed`, the User still exists. Show creation success and the separate sending outcome; direct the person to self-service password setup. Do not retry User creation or offer an Owner resend/reset action.

New Problem Details types:

- `email-auth-unavailable` — 503; setup/recovery is not configured. Preserve the user's form and present service unavailability.
- `first-time-code-invalid` — 410; wrong, expired, consumed, replaced, blocked or ineligible code. Do not distinguish the cause or claim an email exists.
- `password-setup-invalid` — 410; grant cannot be used. Offer fresh password setup.
- `password-reset-invalid` — 410; Reset link cannot be used. Offer Forgot password to request another.

Extend the checked error-code mapping against the new generated enum. Retain all existing types, including `email-taken`, `session-ended` and `login-failed`. Creation retains 401/403/409 and validation 422 behavior. Password policy remains at least six characters, counting characters rather than UTF-8 bytes; do not silently increase it.

## Required screens and actions

### Login and first-time setup

Add “Set up your password” and “Forgot password” entry points on Login. Keep normal email/password login unchanged: there is no OTP on subsequent logins.

The new first-time email links to **`/password-setup`**, with no code or email in the URL. The page lets a person enter their registered email and the six-digit code they already received, or explicitly request/resend instructions. Do not automatically send on page load. Successful verification returns a single-use grant, valid for ten minutes; use it to submit their chosen password, then return to Login without treating the grant as a session.

Codes expire after ten minutes. Only the newest works. Five wrong attempts block verification for that email for ten minutes; resending does not reset failures or the block. Four wrong attempts must still permit a correct code. Code and grant are consumed once. Avoid promising a precise remaining attempt/wait count: the API deliberately does not expose it.

Keep grants/codes out of URLs, telemetry, error reporting and persistent storage. A refreshed page that loses an in-memory grant must provide a self-service way to start again; it must not reuse an already consumed code as authority.

### Forgot password and reset

Forgot password accepts email and displays the generic request result. Every Active User with a password, including the Owner, can use it. Eligible Admin/Staff Users without passwords receive a First-time code instead of a Reset link; the email directs them to the correct flow. The public response does not identify which flow was sent.

Reset emails link to **`/reset-password#token=<opaque-token>`**. Read the fragment client-side, keep the token in memory, and remove it from visible browser history once captured. Do not put it into an API URL/query, browser storage, analytics, referrers or logs. Opening a link must not consume it or change a password: submit only when the person chooses a new password.

Reset links last thirty minutes and only the newest works. Requesting a link preserves the existing password and sessions. Saving the new password ends all sessions on every device, consumes all outstanding setup/recovery credentials, and returns to Login. Clear local authenticated state after success. The completion email contains no password. Its delivery failure does not undo the reset or turn 204 into an error.

### Owner's Users page and installation exception

Switch creation to `create-email-user`. Retain User listing, name/Role/status edits and the `passwordSet` indication. Remove Owner-triggered reset/reissue/copy-link controls from the adopted web experience. Do not replace them with an Owner “email another code” control; people request their own instructions through Login.

The first Owner is the installation exception: retain the existing developer-issued Setup-link page/contract for initial setup, without email or OTP. Public setup/recovery requests for the Owner before initial password setup send no email and retain the generic message. Once setup is complete, Owner recovery uses Forgot password normally. Email stays immutable; lost-mailbox recovery is outside this application.

Deactivation invalidates all credentials and sessions. Reactivating a User cannot revive an old code, grant or link; ask the person to request fresh instructions.

## Shared limits and retry behavior

Setup and recovery share a sixty-second per-email cooldown, five requests per email in a rolling hour and thirty per IP in a rolling hour, across API instances and letter-case variants. Automatic first-time sends participate too. Unknown-address and failed-send requests consume the same reservation. Suppressed requests return the same generic 202; do not expect 429/Retry-After to identify the reason.

Resend UI should not encourage rapid retries. Initial creation remains committed if email sending fails, and self-service retry remains available under the limits. A later request replaces the previous credential even if the previous email arrives late. Do not extend expiry for delayed mail.

## Design and email ownership

API has finished the responsive HTML/plain-text code, reset and completion templates. [Preview gallery and configuration](../email/README.md) contains actual rendered HTML/text and six inspected phone/desktop images. Web must not recreate email templates or introduce an alternative visual direction.

Use web's established design system for its new screens: the read-only reference was `docs/design.md` and `src/index.css` at web commit `0d634468a78e0f137719255d2980b2bfcdf63942`. Use existing current components/tokens in the receiving repo. Forest `#3A6B4F`, Page `#F6F7F4`, Ink `#20261F`, white cards, Figtree/Noto Sans Thai, English UI, existing focus/error/responsive treatments. No new design system is needed.

## Deployment prerequisites

API must receive an activated usable Brevo account, individually verified accessible sender mailbox, `BREVO_API_KEY`, `EMAIL_SENDER_ADDRESS`, trusted HTTPS `WEB_BASE_URL`, and shared `EMAIL_CODE_SECRET` of at least 32 random bytes. Configure secrets only through deployment settings. A Vercel `*.vercel.app` web URL is the page destination, not an email sending domain. No Gmail SMTP or owned domain is assumed; Brevo may rewrite the visible sender address to its provider domain. No paid upgrade or alternate provider is authorised.

No account activation, sender verification, production configuration or real inbox delivery was available during API implementation. Automated tests fake the external provider. Verify real sending and fragment routing separately after authorised deployment. Never ask for secrets in chat or commit them to web.

## Compatibility, cutover and rollback

This is an expansion release: legacy creation, Owner resets and Setup-link operations remain compatible for the existing pinned client during adoption. There is no automatic cutover flag. The new web experience uses versioned creation/recovery and removes its legacy actions; do not claim that the old server endpoints have already retired.

[API ticket #49](https://github.com/thawatchaiUdjan/stock-flow-api/issues/49) owns their later deliberate retirement and legacy non-Owner link invalidation. It remains blocked on the completed API release, web's exact adopted pin and real integration result, and explicit owner cutover authorisation. Report those facts back through this handoff process; API has not posted a web issue.

Keep migration 00010's additive tables during application rollback; do not drop them, restore consumed credentials, or recreate invalidated links. Prefer a forward fix after Users have used new recovery. If reverting API code to the pre-email version is unavoidable, obtain operator authorisation, stop API writes, invalidate outstanding email credentials, deploy the old code, then resume legacy writes. The old implementation cannot invalidate new credentials during later legacy password changes; cleanup prevents those stale credentials becoming usable again after roll-forward. Preserve existing passwords and sessions. No rollback or cleanup was executed during this task.

Old web can remain pinned during preparation, but rolling it back after new Users exist may restore obsolete Owner-reset controls, so coordinate application rollback rather than assuming the UI rollback alone is safe. The later retirement plan must prove that mixed deployments or rollback cannot resurrect old links.

## Receiving-repo acceptance evidence

- Pin the exact API commit, regenerate/check client and error enum, and report that pin.
- Test new User creation and both sending outcomes without exposing/copying credentials or making duplicate Users.
- Test code verification, leading zeroes, replacement, single use, five-attempt boundary, expiry, grant expiry and password validation through the generated client.
- Test generic Forgot password outcomes, unfinished-User email flow, Active Owner recovery and initial Owner no-email installation exception.
- Test reset fragment parsing/history cleanup, no consumption on link open, thirty-minute expiry, newest link, single use, no automatic login and session invalidation across devices.
- Test deactivation/reactivation, shared resend limits and delivery failures with honest response fixtures. Do not make mocks claim real inbox delivery or bypass server limits.
- Verify existing normal login, roles, navigation, User editing and first Owner installation remain usable during migration.
- Follow web's own fast checks and separate API integration workflow, using the same API pin and actual PostgreSQL-backed API. Report the integration result. Mocked browser tests and generation checks alone are not adoption proof.
- After authorised API deployment/configuration, verify all three actual emails in an inbox, sender identity, phone/desktop readability and trusted web routing. Record any email-client-specific limitations honestly.

Keep the work in the web repo on its own reviewable branch/PR. Do not alter API business rules, merge without owner authority, or treat this handoff as permission to skip web's record/workflow gates.
