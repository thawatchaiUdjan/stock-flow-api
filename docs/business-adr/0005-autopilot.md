# 0005: Autopilot: the agent decides and merges, the requirements owner reviews afterwards

Status: Superseded by ADR 0008
Date: 2026-11-07
Affects: api, web

## Context

stock-flow is a practice project ([README](https://github.com/thawatchaiUdjan/stock-flow-context/blob/4c81bd45ff27cca31bd911e5e429b46c273265d2/README.md#project-goal)) with no real requirements behind it. The workflow so far stops for the requirements owner at every step: each answer in a grill, the record pull request, the ticket breakdown, every implementation pull request and every worker's question. For Users & access, every one of its tickets waited on a person. Four modules remain (Products, Stock movements, Orders, Dashboard & reports), and the requirements owner would rather review finished work than unblock it step by step.

## Decision

On 2026-11-07 the requirements owner, thawatchaiUdjan, delegated every decision to the agent in `stock-flow-context` until this ADR is superseded: "approved for now, follow the recommendation; I will review later".

- **Requirements.** The agent writes the rules itself, taking its own recommended answer to every question it would have asked. A module it settles is marked `Status: Confirmed (<date>, agent under ADR 0005)`.
- **Gates.** None of the ✋ gates in `docs/workflow.md` waits for a person. The agent merges the record pull request, publishes the spec and tickets, and starts the workers.
- **Merging.** The coordinator merges each implementation pull request once its pipeline is green. Workers still never merge.
- **Questions.** The coordinator answers workers' questions itself, recording a business answer in the record first, as before.
- **Review queue.** Every decision the agent makes on the owner's behalf is listed in [`docs/review-queue.md`](https://github.com/thawatchaiUdjan/stock-flow-context/blob/b2661bed56b38fcd23030a77eebe4872d1f826bc/docs/review-queue.md), with a link to where it lives. The owner reviews from there.

The agent still stops for what only a person can do: a step that needs the owner's credentials or a third-party dashboard, a pipeline that stays red after two fixes, or a change that could lose stored data.

## Consequences

- The four modules are built without waiting on a person, and the owner reviews them at once afterwards.
- A wrong decision can be built on before anyone sees it. Each module is kept to the smallest version that does its job, and later modules lean on earlier ones only through their rules, so a reversal stays an ordinary ADR.
- The owner disagrees with a decision through `/grill-with-docs`, which amends it by an ADR as usual. Nothing in the queue is binding on the owner.
- api and web: merges to `main` deploy, so a merged pull request reaches the deployments without a person seeing it. Fine while nothing real is stored.
- api and web: a ticket's *Authorised by* line names this ADR, so a worker can tell the work was delegated.

## Alternatives considered

- Keep every gate: the owner unblocks some forty steps by hand, which the owner rejected.
- Drop only the implementation gates and keep grilling the owner: the owner has no real requirements to give.
- Let workers merge their own pull requests: one coordinator merging keeps the order of merges, and the check that the pipeline is green, in one place.
