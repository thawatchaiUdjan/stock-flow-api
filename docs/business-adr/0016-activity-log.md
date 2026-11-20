# 0016: An Activity log records every change

Status: Accepted
Date: 2026-11-20
Affects: api, web
Authorised by: Requirements owner (thawatchaiUdjan), `/grill-with-docs` session of 2026-11-20, as in business ADR 0014. The owner asked what should be logged for an audit and delegated the decision. Merge not authorised.

## Context

The Login log records logins and logouts only, and Users & access, *Out of scope*, excludes "a log of anything other than logins and logouts, such as who changed whose Role". Stock movements already record every stock change, and an Order records who created, completed or cancelled it. But a Product's price change, a User's Role change, a Deleted record or a Customer's account change leaves no trace anyone can see.

## Decision

- **One Activity log** of changes to Products, Orders, Users and Customers, beside the Login log, which keeps logins.
- **Each entry says** when, who (a User, a Customer, the system or the installation, with their name, email and Role as they were), what was done, to what (its kind and its name, SKU, number or email as they were), what changed (each changed field's value before and after), and the IP address and browser or device.
- **What is logged:** every change in [Activity log](../requirements/activity-log.md) R4. Receives and Adjustments are not repeated: Stock movements already are their permanent record.
- **What is never logged:** passwords, codes, links, tokens, session identifiers and card details; anything only viewed; and requests that were refused.
- **No change without its entry.** A change and its entry are saved together or not at all.
- **Permanent and private.** Entries are never edited or deleted, are kept indefinitely, and only the Owner sees them.

## Consequences

- Add the Activity log module and glossary term; remove the log exclusion from Users & access, *Out of scope*; add the Activity log page to Users & access R11 and Navigation & lists.
- api: every operation that changes these records writes its entry in the same transaction. Storage grows with use and is never trimmed; revisit if the free database fills.
- Changes made before the Activity log existed have no entries.

## Alternatives considered

- Extending the Login log: mixes access events with changes and blurs its meaning.
- Logging every request, reads included: far more data, mostly noise, and personal data nobody needs.
- Repeating Stock movements in the Activity log: two records of the same fact that could disagree.
- Letting Admins see it: they make most of the changes it records.
