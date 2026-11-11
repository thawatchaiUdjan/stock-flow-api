# 0004: The Owner's name is not changed in the system

Status: Accepted
Date: 2026-11-06
Affects: api, web

## Context

R35 says the Owner can change a User's name, with no exception, while R18 protects only the Owner's own status and Role. The api ticket for managing Users therefore asked whether the Owner may change their own name. The Owner is created by a developer at installation (R9, R34), with the name given then.

## Decision

The Owner's own name cannot be changed in the system, like their status and Role. A wrong name is corrected by a developer in the database.

## Consequences

- api: Update User refuses every change to the Owner (status, Role and name) with the same error.
- web: the Users list offers no edit action on the Owner's row.
- Amends R35 in `users-and-access`: the Owner changes the name of other Users only.
- A typo in the Owner's name at installation needs a developer.

## Alternatives considered

- Let the Owner change their own name only: more consistent with R18's narrow wording, but the requirements owner wants the Owner's identity fixed at installation.
