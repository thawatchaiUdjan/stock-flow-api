# Requirements

One file per domain module: `docs/requirements/<module>.md`. Create a module's file when there is real content for it, not before.

## Shape of a module file

```md
# <Module>

Status: Draft | Confirmed (YYYY-MM-DD, <who confirmed>) | Amended by ADR NNNN

## Purpose
Who uses this module and what it is for, in two or three sentences.

## Rules
- **R1** <one rule, stated as fact>
- **R2** <one rule>

## Out of scope
What this module deliberately does not do.

## Open questions
| # | Question | Asked of | Since |
| --- | --- | --- | --- |
| Q1 | <a question nobody has answered yet> | <who can answer> | YYYY-MM-DD |
```

## Rules

- **Rule IDs are stable.** Tickets, ADRs and code comments cite a rule as `<module> R3`. Never renumber; a removed rule keeps its number with a note (`R3 removed by ADR 0009`).
- **One rule, one line of intent.** If a rule needs a paragraph of justification, the justification is an ADR.
- **The record pull request sets the status.** A file starts as `Draft` while it is being grilled; the pull request that settles its rules sets `Status: Confirmed (YYYY-MM-DD, <who>)`, and merging it is the confirmation. `/to-tickets` must not mark implementation ready while a cited module is still `Draft`.
- **A confirmed rule changes only through an ADR**, which the `Status:` line then names.
- **Open questions are the honest part of the file.** Anything not yet confirmed goes there, never into Rules. An answered question moves into Rules (or an ADR) and leaves the table.

## Ownership and imported citations

This is the API-owned record. The one-time import and source hashes are in [migration provenance](../migrations/context-ownership.md). Imported rule IDs, statuses, confirmations and open questions retain their meaning. Unqualified ADR numbers in imported module files refer to [business ADRs](../business-adr/README.md), not technical ADRs. New citations qualify the namespace.

All seven modules were retained so cross-module rules and existing historical citations stay intelligible. Navigation and date-control presentation rules describe the existing product; their import does not assign UI implementation to API or claim that web has adopted this workflow. API implements server obligations; web owns its own record, plan and UI. Request cross-repo changes through linked issues.
