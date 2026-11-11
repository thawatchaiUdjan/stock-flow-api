# Domain docs

Before exploring or planning, read local `GLOSSARY.md`, relevant `docs/requirements/<module>.md`, and applicable business and technical ADRs. The record travels with this checkout. A missing required rule is an owner question, never permission to invent one.

## Ownership and namespaces

This repo owns its API record. `/grill-with-docs` uses `grilling` and `domain-modeling` to resolve terms and decisions locally. These paths and numbering rules override the skills' default `docs/adr/` path for business decisions.

| Kind | Path | Citation |
| --- | --- | --- |
| Vocabulary | `GLOSSARY.md` | Glossary term |
| Product rules and questions | `docs/requirements/<module>.md` | `<module> R3` |
| Business decisions, including workflow ownership | `docs/business-adr/NNNN-slug.md` | `business ADR 0007` |
| Technical decisions | `docs/adr/NNNN-slug.md` | `ADR 0007` or `technical ADR 0007` |

Scan only the target directory for the highest number, then increment. Never renumber an existing ADR or rule. Legacy `context ADR NNNN` resolves to the same-number imported business ADR. Bare ADR numbers inside imported business ADRs and requirement files also mean business ADRs, except explicitly qualified API/web technical references. Bare ADR citations in existing code and technical ADRs retain their technical meaning. [Provenance and exceptions](../migrations/context-ownership.md).

A business decision would survive rewriting the API in another stack. A technical decision chooses how to implement it. Write an ADR when hard to reverse, surprising without context and a real trade-off. A change to a confirmed rule always needs a business ADR, even if small. Other settled rules go in the module file; open decisions go in `Open questions`.

## ADR format

This format overrides `domain-modeling`'s short template:

```md
# NNNN: <Short title>

Status: Proposed | Accepted | Superseded by <qualified ADR>
Date: YYYY-MM-DD
Affects: api[, web]
Authorised by: <owner decision or explicit task-scoped delegation, with evidence>

## Context
## Decision
## Consequences
## Alternatives considered
```

`Affects` identifies impacted repos; it does not grant permission to edit them or bind web to adopt the decision. Request affected web work directly through linked issues.

The record PR sets acceptance/confirmation and cites the owner decision; the merged record is authoritative. Accepted, merged ADR decision text is immutable. Change course with a new ADR naming what it supersedes; only the old ADR's `Status:` line may change. Imported decision text and historical authorisation remain evidence, not fresh delegation. Business ADR 0010 supersedes imported workflow ADRs 0005 and 0008 for this API only.

## Glossary and conflicts

Use `GLOSSARY.md` terms in modules, types, functions, tests, endpoints and tickets; avoid its rejected synonyms. Follow `domain-modeling`'s `GLOSSARY-FORMAT.md`: definition and `_Avoid_` synonyms, without implementation detail, rules or history.

Never write an assumed requirement. Resolve missing vocabulary through `/grill-with-docs`. Discover facts yourself; ask the requirements owner for decisions unless explicitly delegated. Keep unanswered questions, author and date in the module file. If code, a ticket or a proposal contradicts a rule or ADR, surface the conflict before changing affected behaviour. Pause only dependent work.
