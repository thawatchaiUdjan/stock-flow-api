# Domain Docs

This repo **reads** the domain documentation; `template-context` owns it.

## Before exploring, read these

- **`../template-context/GLOSSARY.md`**: the shared vocabulary.
- **`../template-context/docs/requirements/`**: the module files that touch the area you're working in.
- **`../template-context/docs/adr/`**: business decisions that touch it. Each ADR's `Affects:` line says whether it applies to this repo.
- **`docs/adr/`** here: technical decisions.

If a file doesn't exist yet, proceed silently. If `template-context` itself can't be found, stop and say so: the record is never optional.

## Never create these here

- **No `GLOSSARY.md` in this repo.** A second glossary is two vocabularies. A missing term is an open question for `template-context`.
- **No business ADRs here.** The test is in `template-context`'s `AGENTS.md`: a decision that would survive a rewrite in another stack belongs there.

## Technical ADRs

`docs/adr/NNNN-slug.md`, numbered by scanning `docs/adr/` for the highest number. Create the directory with the first ADR.

Write one only when the decision is hard to reverse, surprising without context, and the result of a real trade-off: the language, the framework, the database, the deployment target, a deliberate departure from the obvious approach.

```md
# NNNN: <Short title>

Status: Proposed | Accepted | Superseded by ADR NNNN
Date: YYYY-MM-DD

## Context
## Decision
## Consequences
## Alternatives considered
```

An Accepted, merged ADR is never edited: changing course is a new ADR that supersedes it.

## Use the glossary's vocabulary

Name modules, types, functions, tests and endpoints with the glossary's terms, never with the synonyms it lists under `_Avoid_`.

## Flag ADR conflicts

If your change contradicts an ADR, here or in `template-context`, say so explicitly rather than silently overriding it.
