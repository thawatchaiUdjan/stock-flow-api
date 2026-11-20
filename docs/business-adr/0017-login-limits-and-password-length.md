# 0017: Failed logins are limited, and passwords keep a 6-character minimum

Status: Accepted
Date: 2026-11-20
Affects: api, web
Authorised by: Requirements owner (thawatchaiUdjan), `/grill-with-docs` session of 2026-11-20, as in business ADR 0014. The design presented to the owner included these limits for Users and Customers, and the owner approved writing it. On review the same day, the owner kept the 6-character minimum for Users and Customers alike and rejected raising it to 8. Merge not authorised.

## Context

Nothing limits failed logins, and a password needs only 6 characters (Users & access R19). The [security notes](../security-notes.md) rate the first High, and Users & access holds both as open questions 1 and 2. The Storefront puts a public sign-up in front of anyone on the internet, so failed logins can no longer go unlimited, and Customers need the same password rule as Users.

## Decision

- **Per email.** After 5 failed logins for one email within 15 minutes, logins for that email are blocked until 15 minutes after the last failure, even with the right password. This applies to any email entered, known or not.
- **Per address.** After 50 failed logins from one IP address within an hour, logins from it are blocked until fewer than 50 remain in the last hour.
- **What a blocked person sees.** The refusal says logins are temporarily blocked and when to try again, and nothing about the email. Forgot password still works. The Login log records each blocked attempt.
- **Users and Customers alike**, counted separately for StockFlow and the Storefront.
- **Passwords** keep the 6-character minimum, for Users and Customers alike, and gain a maximum of 128 characters. No existing password is affected.

## Consequences

- Amend Users & access R19 and add R55–R57; remove the lockout exclusion from *Out of scope* and close open questions 1 and 2. Customers R9 and R11 follow the same rules.
- Someone can block a known email for 15 minutes by failing on purpose; Forgot password is not blocked.
- Six characters remain weak; the login limits are what slow guessing. No list of refused common passwords is added. Revisit both before real data goes in.

## Alternatives considered

- A lock the Owner clears: a stranger could lock the Owner out, with nobody left to clear it.
- Per address only: an attacker rotating addresses is not slowed.
- An 8-character minimum: proposed by the agent, rejected by the owner, who wants the same 6 characters as today.
- Forcing existing passwords to change: sends every User through recovery for a practice project.
