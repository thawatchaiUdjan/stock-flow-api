# 0008: Choose the work mode per task and remove the review queue

Status: Accepted
Date: 2026-11-09
Affects: api, web

## Context

ADR 0005 delegates all decisions indefinitely and requires a growing review queue. The requirements owner asked to remove that queue and choose between Autopilot and the original approval workflow before each task, with Autopilot as the default.

## Decision

This ADR supersedes ADR 0005. Before starting each new task, ask the owner to select Autopilot (default) or Approval workflow and wait for the answer. A mode explicitly named in the request is already selected. The default is a suggested choice, not an unanswered approval. Read-only questions do not require selection.

The choice covers the entire task, including resumed sessions and workers. Carry the mode, scope and owner selection in tickets and handoffs. Ask again for a new task or when the selection cannot be recovered. An explicit mode switch applies to remaining work.

Autopilot delegates decisions and approval gates to the agent for that task. The coordinator merges implementation pull requests only after required reviews and a green pipeline; workers never merge. Stop for credentials, third-party dashboards, a pipeline still red after two fixes, or changes that could lose stored data. Approval workflow retains the original owner questions and approval gates.

Remove the review queue without creating an archive or replacement checklist. Decisions remain in requirements, ADRs and pull requests. Existing rules are not revoked by removing the queue. Unresolved security findings remain in security notes and module open questions. Report blockers and excluded scope in the pull request and task summary.

## Consequences

- api and web workers inherit task authorisation rather than a permanent delegation. Mode selection belongs to the coordinator, not each worker.
- Autopilot requires one choice at the start; Approval workflow requires subsequent owner approvals too.
- There is no accumulating obligation to review agent decisions afterwards. Git history retains the deleted queue; removing it does not mark its contents as reviewed or fixed.

## Alternatives considered

- Keep a permanent Autopilot delegation: does not offer the requested choice before work.
- Archive reviewed queue items: still creates the review bookkeeping the owner wants removed.
- Proceed with the default after no answer: contradicts asking the owner before work begins.
