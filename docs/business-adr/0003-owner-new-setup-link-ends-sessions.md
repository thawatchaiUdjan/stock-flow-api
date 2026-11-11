# 0003: Setting the Owner's password through a new Setup link ends their sessions

Status: Accepted
Date: 2026-11-06
Affects: api

## Context

R34 says the Owner's password is not reset in the system: a developer issues the Owner's Setup link. R31 ends a User's sessions when the Owner resets their password, but R34 exempted the Owner, so the record was silent on the Owner's sessions. If the Owner asks for a new Setup link because their password leaked, an attacker's session would stay live for up to 7 days (R21).

## Decision

When the Owner sets a password through a Setup link, all of the Owner's sessions end at once, on every device. The Owner logs in again with the new password.

## Consequences

- api: setting a password through the Owner's Setup link deletes all of the Owner's sessions, in the same transaction that uses up the link. A request with an ended session is answered as in R26.
- web: nothing new; the person is already sent to Login after setting a password (R30).
- Amends R34 in `users-and-access`.

## Alternatives considered

- Leave the Owner's sessions alive: an attacker who got in with a leaked password keeps access for up to 7 days.
