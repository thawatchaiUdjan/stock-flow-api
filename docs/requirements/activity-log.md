# Activity log

Status: Confirmed (2026-11-20, requirements owner's task-scoped delegation in business ADR 0016)

## Purpose

The Owner can see who changed what, and when, across Products, Orders, Users and Customers. The Login log keeps logins; Stock movements keep stock changes; the Activity log keeps everything else ([business ADR 0016](../business-adr/0016-activity-log.md)).

## Rules

- **R1** The Activity log records each change listed in R4 as one entry.
- **R2** An entry has the time; who made the change (a User, a Customer, the system, or the installation) with their name, email and, for a User, Role as they were then; the action; what it changed (its kind and its name, SKU, Order number or email as it was then); for an edit, each changed field's value before and after; and the IP address and browser or device, when a person made it.
- **R3** A change and its entry are saved together: a change is never saved without its entry, and an entry is never saved for a change that failed.
- **R4** These changes are recorded:

  | Of | Changes |
  | --- | --- |
  | Products | Added; edited, with each changed field before and after; Archived; restored; Deleted |
  | Orders | Created (In-shop) or placed (Online); Paid; Completed; Cancelled, saying by whom or by the system; Refunded; Deleted |
  | Users | Created, with their Role; name changed; Role changed; made Inactive or Active; Deleted; first password set; password reset; Setup link issued |
  | Customers | Signed up or created by the Shop; first password set; name, phone number or saved Delivery address changed; password reset; made Inactive or Active; Deleted |

- **R5** Receives and Adjustments are not repeated in the Activity log: Stock movements are their record (Stock movements R8).
- **R6** An entry never holds a password, a code, a link, a token, a session identifier or card details.
- **R7** Viewing a page or record, and requests that were refused, are not recorded. Failed and blocked logins are in the Login log (Users & access R36).
- **R8** Entries cannot be edited or deleted, and are kept indefinitely.
- **R9** Only the Owner reaches the Activity log (Users & access R11).
- **R10** The Activity log lists the newest entries first and can be filtered by who made the change, by the kind of record, by action and by a range of the Shop's days (Dashboard & reports R1). It pages like the Login log (Navigation & lists R6, R8).
- **R11** An entry about a Deleted record still shows it, by the name, SKU, number or email recorded in the entry.

## Out of scope

- A history panel on each Product, Order, User or Customer.
- Exporting the Activity log.
- Alerts when something is changed.
- Entries for changes made before the Activity log existed.

## Open questions

None. Every rule here was decided by the agent under the requirements owner's task-scoped delegation of 2026-11-20 (business ADR 0016).
