# 0017: Failed logins are limited, and new passwords have 8 characters or more

Status: Accepted
Date: 2026-11-20
Affects: api, web
Authorised by: Requirements owner (thawatchaiUdjan), `/grill-with-docs` session of 2026-11-20, as in business ADR 0014. The design presented to the owner included these limits for Users and Customers, and the owner approved writing it. Merge not authorised.

## Context

Nothing limits failed logins, and a password needs only 6 characters (Users & access R19). The [security notes](../security-notes.md) rate the first High, and Users & access holds both as open questions 1 and 2. The Storefront puts a public sign-up in front of anyone on the internet, so neither can wait.

## Decision

- **Per email.** After 5 failed logins for one email within 15 minutes, logins for that email are blocked until 15 minutes after the last failure, even with the right password. This applies to any email entered, known or not.
- **Per address.** After 50 failed logins from one IP address within an hour, logins from it are blocked until fewer than 50 remain in the last hour.
- **What a blocked person sees.** The refusal says logins are temporarily blocked and when to try again, and nothing about the email. Forgot password still works. The Login log records each blocked attempt.
- **Users and Customers alike**, counted separately for StockFlow and the Storefront.
- **Passwords** set from now on have 8 to 128 characters. Existing shorter passwords still work until replaced.

## Consequences

- Amend Users & access R19 and add R55–R57; remove the lockout exclusion from *Out of scope* and close open questions 1 and 2. Customers R9 and R11 follow the same limits.
- Someone can block a known email for 15 minutes by failing on purpose; Forgot password is not blocked.
- No list of refused common passwords is added.

## Alternatives considered

- A lock the Owner clears: a stranger could lock the Owner out, with nobody left to clear it.
- Per address only: an attacker rotating addresses is not slowed.
- Forcing existing passwords to change: sends every User through recovery for a practice project.
