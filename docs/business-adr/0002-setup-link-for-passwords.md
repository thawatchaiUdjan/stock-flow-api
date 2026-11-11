# 0002: Users set their own password through a one-time Setup link

Status: Accepted
Date: 2026-11-06
Affects: api, web

## Context

Every User must set their own password, so that nobody else ever knows it. The system sends no email yet, so it has no way to confirm that the person setting a password is the person the email belongs to. The app is on the public internet and its repos are public.

## Decision

A password is set only through a Setup link: unguessable, single use, valid for 24 hours. The system issues it when a User is created or reset, and shows it once to the Owner, who passes it on by hand. A developer issues the Owner's own. There is no "Forgot password" on Login until email exists.

## Consequences

- A User who forgets their password depends on the Owner; the Owner depends on a developer.
- The email feature only changes how the link travels: the system sends it instead of the Owner. A one-time code can be added as a further step.
- api: issue, store and expire Setup links, and let only the newest one work. A link must be stored so that reading the database does not reveal it.
- web: a page that sets the password from a link, and a way for the Owner to copy a link once.

## Alternatives considered

- Set a password at first login with only the email: anyone who knows the email could take the User first.
- A temporary password typed by the Owner: the Owner would know it, which the requirements owner rejected.
- "Forgot password" on Login without email: anyone could take over any User, including the Owner.
