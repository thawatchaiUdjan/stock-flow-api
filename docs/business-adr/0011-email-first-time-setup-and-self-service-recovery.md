# 0011: Email first-time setup and self-service password recovery

Status: Accepted
Date: 2026-11-11
Affects: api, web
Authorised by: Requirements owner's 2026-11-11 grill-with-docs interview (Q1–Q20) and final "yes" to the consolidated design. Continue in Autopilot through authorised stages; merges are not authorised.

Record and confirmation evidence: [API issue #44](https://github.com/thawatchaiUdjan/stock-flow-api/issues/44).

## Context

[Users & access](../requirements/users-and-access.md) R27–R34 currently require manually shared Setup links and Owner-triggered resets, with developer recovery for the Owner. Business ADRs [0002](0002-setup-link-for-passwords.md) and [0003](0003-owner-new-setup-link-ends-sessions.md) describe those flows. Email and first-time codes are currently outside scope.

The owner requested first-time OTP and designed password-recovery emails, then explicitly rejected Owner-triggered resets: "reset password can't do by action now, need to do by them self by click 'forgot password'". The owner requires the existing design system, no installation email for the first Owner, and a written web handoff after API work rather than web issue requests.

This record consolidates the confirmed interview answers. These changes become authoritative when the record PR merges; dependent implementation cannot begin from an unmerged record.

## Decision

### First-time password setup

- Only the Owner creates Admin and Staff Users; self-signup remains unavailable.
- Creating an Admin or Staff User automatically emails a First-time code. The User verifies the code, chooses their own password, and returns to Login. Later logins use email and password without OTP.
- Login offers password setup with email entry and resend for existing Active Users without passwords, except the initial Owner before installation setup completes.
- First-time codes contain six digits and expire after 10 minutes. Only the newest code works; resending invalidates the previous code.
- Five incorrect code attempts block code verification for that email for 10 minutes. Resending cannot bypass the limit. This is separate from the existing unresolved policy for ordinary password-login failures.
- Successful verification grants 10 minutes to set the first password. The grant is single-use and does not create a logged-in session.
- The first Owner keeps the existing developer-issued, single-use, 24-hour Setup link for installation. Initial Owner setup sends no email or OTP, including through public setup or recovery requests before that setup completes.

### Self-service password recovery

- Every Active User with a password, including the Owner, requests recovery through Forgot password. Remove the Owner-triggered password-reset action.
- Recovery emails a single-use Reset link, valid for 30 minutes. Issuing a replacement invalidates the previous link.
- Requesting recovery preserves the current password and sessions. Saving the new password ends all sessions, invalidates outstanding recovery credentials, sends a completion notification, and returns the User to Login without automatic login.
- Completion-notification failure does not undo a successful password change.
- Unknown and Inactive Users receive no email. An eligible non-Owner User without a password receives a First-time code rather than a Reset link. The initial Owner exception above takes precedence.
- Public email requests return the same eligibility-neutral message, without revealing whether a User exists or is eligible.

### Limits, failure and lifecycle

- Setup and recovery share email-request limits: one send per 60 seconds, five per email per hour, and 30 per IP per hour. The owner delegates exact windows, counter semantics and retry handling within those limits.
- If initial email sending fails, the User remains created without a password and can retry through password setup. The Owner sees creation success and sending failure separately; public requests keep their generic response.
- Deactivation invalidates codes, verification grants, Reset links and sessions. Reactivation requires fresh instructions.
- Codes, links and verification grants are single-use and become invalid after password setup or reset.
- Email addresses remain immutable; the existing password policy remains unchanged. Lost mailbox access requires developer assistance outside the application. No new in-app email-change or emergency-recovery flow is authorised.

### Email design and ownership

- API owns and finishes the email layout, templates, rendering verification and delivery. Web owns its application screens.
- First-time code, Reset link and reset-completion emails use English, StockFlow branding, responsive HTML and a plain-text alternative.
- Follow the existing light design direction: Forest `#3A6B4F`, Page `#F6F7F4`, White surfaces and Ink `#20261F`. Preserve the Figtree/Noto Sans Thai direction where supported, adapting fallbacks and layout for email clients.
- Visual reference: web commit `0d634468a78e0f137719255d2980b2bfcdf63942`, [design direction](https://github.com/thawatchaiUdjan/stock-flow-web/blob/0d634468a78e0f137719255d2980b2bfcdf63942/docs/design.md) and [tokens](https://github.com/thawatchaiUdjan/stock-flow-web/blob/0d634468a78e0f137719255d2980b2bfcdf63942/src/index.css). Read remotely; never edit the sibling checkout.

## Consequences

- Amend users-and-access R27–R32 and R34 without renumbering, add R41–R49, and remove the superseded scope exclusions. R33 remains unchanged. Supersede business ADR 0002 for normal User setup/recovery while retaining the initial Owner installation exception. Preserve business ADR 0003's intent that changing the Owner's password ends all Owner sessions.
- Publish the generated contract with explicit compatibility/versioning, migration and rollback details. The current contract exposes Setup tokens and an Owner-reset operation; do not silently change their meanings. Preserve pinned-client compatibility until an explicitly documented cutover. Production activation that depends on web adoption remains gated on web readiness.
- After API work finishes, write a web handoff in this repo with changed flows, affected operations/errors, exact API publication commit, design references, cutover/rollback instructions and integration checks. Do not send web issue requests. Do not claim web adoption without its confirmed pin and integration result.
- Continue authorised stages in Autopilot following the completed-design confirmation. Remaining technical choices are delegated within these rules; new business decisions return to the owner. A merged authoritative record remains a prerequisite for dependent implementation. No merge authority was granted.

### Delivery plan and prerequisites

The owner has only a Vercel-provided deployment URL, no owned sending domain, and Gmail is unavailable. Resend with an owned sending domain and Gmail SMTP are excluded from this delivery plan.

The owner accepted Brevo in Q20 conditionally: "yes if it can use". Use its free HTTP transactional-email service subject to account activation and live validation, with sender name StockFlow. Brevo supports [individual mailbox verification](https://help.brevo.com/hc/en-us/articles/208836149-Create-a-new-sender-From-name-and-From-email) and [temporary provider-domain replacement](https://help.brevo.com/hc/en-us/articles/16045394674066-Troubleshooting-Domain-authentication) for unauthenticated/free-provider sender addresses. Recipients may therefore see an address on `brevosend.com`. Its [free plan](https://help.brevo.com/hc/en-us/articles/208589409-About-Brevo-s-pricing-plans) currently allows 300 sends per day. These provider facts were checked 2026-11-11; they are not product guarantees.

An accessible sender inbox, activated provider account, API key, verified sender and trusted deployed web base URL are deployment prerequisites. Configure credentials through deployment secrets, never chat or committed files. Account eligibility and live delivery are unverified; implementation and template work can proceed independently after the authoritative record merges. No paid upgrades are authorised. If Brevo cannot activate or deliver for this account, surface that blocker rather than silently selecting a different provider.

The Vercel web URL is the destination for reset/setup pages; it is not an owned email sending domain.

## Alternatives considered

- Keep Owner-triggered resets and email the result: explicitly rejected.
- Require an OTP on every login: outside the first-time-only scope.
- Retain developer-only normal Owner recovery: replaced by emailed self-service recovery after installation. Lost-mailbox assistance remains outside the application.
- Create a separate visual direction for email: rejected in favour of the existing design system.
- Buy a sending domain or paid email plan: not authorised.

## Open questions

None within the confirmed product scope. Brevo account activation, sender verification, live delivery and web adoption remain external prerequisites to validate, not assumed successes. Any provider substitution or new product rule requires an owner decision.

The numerical security limits are owner-selected product rules, informed by [OWASP recovery guidance](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html), not numeric requirements imposed by that guidance. Ordinary-login throttling and password-policy questions remain separately recorded in users-and-access.
