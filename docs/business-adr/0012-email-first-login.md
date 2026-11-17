# 0012: Email-first Login reveals what an email may do next

Status: Accepted
Date: 2026-11-17
Affects: api, web
Authorised by: Requirements owner's decisions of 2026-11-17, stated in the task brief for [API issue #53](https://github.com/thawatchaiUdjan/stock-flow-api/issues/53) and already confirmed on the web side in [web domain ADR 0011](https://github.com/thawatchaiUdjan/stock-flow-web/blob/main/docs/domain/adr/0011-email-first-login.md) ([web #67](https://github.com/thawatchaiUdjan/stock-flow-web/pull/67)). The owner answered the API's grilling questions on throttling, unsent codes and the Login log the same day. Approval mode; merge not authorised.

## Context

Login asks for an email and a password together. First-time Admin and Staff Users must find a separate "Set up your password" action. The owner wants Login to ask for the email first and route each person to the right next step. [Users & access](../requirements/users-and-access.md) R5 and R44 forbid revealing whether an email matches a User, its status, or whether a password is set, so the change is a deliberate trade-off. Web has requested the operation in API issue #53. Web's own record is a request, not authority here; this ADR adopts the rules for the API.

## Decision

Login's email step asks the API what an email may do next:

| Account | Answer | Side effect |
| --- | --- | --- |
| Active User with a password, including the Owner | password step | none |
| Active Admin or Staff User without a password | code step | sends a First-time code within R43's limits |
| Unknown email, or Inactive User | one identical "no account" answer | none |
| First Owner before installation setup | use the installation Setup link | sends nothing (R34) |

- The owner accepts user enumeration for this practice project: anyone can learn whether an email belongs to an Active User and whether it has a password. Inactive status is never distinguished from an unknown email.
- The API limits the email step per IP address, high enough not to block normal logins from one shop network. The owner delegated the numbers; the API sets **100 email checks per IP address per rolling hour**.
- Going over that limit answers a distinct, published "limited" error that says when to retry, without revealing anything about the email.
- When the answer is the code step but no code goes out (R43's send limits apply, or delivery fails), the answer is still the code step, and it says whether a code was sent, limited or failed. A previous unexpired code keeps working, and the User can resend later.
- The email step is not a login attempt: the Login log (R36) does not record it.
- The password step keeps its single failure message (R5). Public setup and recovery requests keep their eligibility-neutral message (R44).

## Consequences

- Amend users-and-access R2, R5, R29 and R44 without renumbering, and add R50–R53. Rule numbers match web's R50–R53; web's R54 (code entry UI) has no API counterpart.
- API publishes a new, additive public operation in the generated contract, with a new error code for the limit. Existing operations keep their meanings, so the pinned web client is unaffected. Web integration waits for the published contract commit.
- An email check that leads to the code step uses the shared R43 send allowance; other answers do not.
- The enumeration trade-off, like the no-lockout decision and R19's minimum, needs revisiting before any real data goes in.

## Alternatives considered

- Two-step Login that never reveals the account (always a password step with a "first time?" link): keeps R5 and R44 but does not route first-time Users; the owner chose the real check.
- Revealing Inactive status separately: leaks more than existence for little benefit; rejected by the owner.
- A plain 429 without a code when limited: web could not tell it from other limits; not chosen.
- Answering an error when a code could not be sent: hides that a previous code may still work; not chosen.
- Recording email checks in the Login log: an email check is not a login attempt; not chosen.
