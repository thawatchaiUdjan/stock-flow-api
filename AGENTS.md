# AGENTS.md

## What this repo is

The backend for stock-flow. This repo owns its API requirements, glossary, business and technical decisions, specs, tickets, implementation and published API contract.

| Repo | Role | Remote (`owner/repo`) |
| --- | --- | --- |
| `stock-flow-api` | **This repo.** API record, workflow, backend and contract | `thawatchaiUdjan/stock-flow-api` |
| `stock-flow-web` | Owns web planning and implementation; consumes a pinned API contract | `thawatchaiUdjan/stock-flow-web` |
| `stock-flow-context` | Historical source only; no checkout or approval service required | `thawatchaiUdjan/stock-flow-context` |

Work only in this repository. Never edit a sibling checkout. Start from `main` on a branch; prepare a reviewable PR. Never merge without explicit owner authorisation.

For planning, record changes, specs, tickets and cross-repo requests, follow `docs/workflow.md`. Existing owner authorisation carries through the task; do not ask again for an action already in scope. A new business decision still needs the requirements owner's answer or explicit task-scoped delegation. Historical Autopilot grants do not authorise new work or merges.

## Stack and commands

Project goal: a practice project for frontend development ([README, *Project goal*](README.md#project-goal)). Here that means: prefer the simplest thing that works.

`/implement` runs typechecking and tests (which ones: `docs/agents/testing.md`) with these commands; never guess others. All of them run from the repo root, in bash and in PowerShell.

| | |
| --- | --- |
| Language / runtime | Go 1.27 (ADR 0001), on Vercel's Go Framework Preset in `sin1` (ADR 0008) |
| Framework | Huma v2 on chi v5 (ADR 0002); pgx v5 + sqlc for SQL (ADR 0003); goose v3 migrations (ADR 0007) |
| Database | PostgreSQL 17 on Supabase Free, Singapore (ADR 0005); app via the transaction pooler (ADR 0006) |
| Install dependencies | `go mod download` (sqlc and goose are `go tool`s, ADR 0004: nothing to install globally) |
| Run locally | Copy `.env.example` to `.env` and fill it in, then `go run ./cmd/api` |
| Typecheck / build | `go build ./...` and `go vet -tags slow ./...` |
| Run the fast layer | `go test ./...` |
| Run one slow test by name | `go test -tags slow -run '^TestName$' ./internal/<package>/` |
| Run everything (CI) | `go test -tags slow ./...` (`ci.yml` also runs gofmt, vet, and the generated-code check) |
| Lint / format | `gofmt -l .` lists unformatted files (`gofmt -w .` fixes them); `go vet -tags slow ./...` |
| Regenerate sqlc code and `openapi.yaml` | `go generate ./...`, then commit the result; CI fails if it is stale |
| New migration | `go tool goose -dir db/migrations -s create <slug> sql` (`-s`: sequential `NNNNN_` numbers, not timestamps) |
| Migrate a database by hand | `go tool goose -dir db/migrations postgres "<MIGRATION_DATABASE_URL>" up` (production is migrated by `deploy.yml` only) |
| Issue the Owner's Setup link | `go run ./cmd/install -name "<name>" -email "<email>"` the first time, then `go run ./cmd/install`; uses `POSTGRES_URL` and prints a token (`users-and-access` R34) |

Coding standards: `CODING_STANDARDS.md`. Technical decisions: `docs/adr/`. Business decisions: `docs/business-adr/`.

**Logs:** Vercel Hobby keeps runtime logs for **1 hour**. When debugging production, reproduce first, then read the logs in the Vercel dashboard straight away.

## Where the rules come from

- **Implement from the local record:** `docs/requirements/`, `GLOSSARY.md` and `docs/business-adr/`, not a ticket's summary or web code. Read `docs/agents/domain.md` before changing the record.
- Never invent requirements. Put unresolved decisions under the module's `Open questions`; discover facts yourself. The requirements owner (thawatchaiUdjan) decides business rules unless explicitly delegated for this task.
- Preserve rule IDs. Cite enforced rules as `// <module> R3` or `// business ADR 0007`. Existing `// context ADR 0007` citations resolve to imported business ADR 0007; bare `ADR 0007` still means this repo's technical ADR. See `docs/migrations/context-ownership.md` for provenance and namespace mapping.
- **If the record, ticket and code disagree, surface the conflict before editing the affected behaviour.** Ask the owner; do not choose silently. Block dependent work only; independent work may continue.
- Confirmed rules change through a business ADR and record PR. Historical workflow instructions in imported ADRs 0005 and 0008 are superseded for this API by business ADR 0010; their past confirmations remain valid.

## Tickets

Specs and implementation tickets live in this repo (see `docs/agents/issue-tracker.md`). A ticket is ready only when it links the merged local record and spec, names the owner authorisation and scope, defines acceptance criteria and test seams, and identifies genuine blockers. Legacy context specs remain historical evidence; carry the needed acceptance criteria locally when resuming an old ticket.

A cross-repo request is information and permission to communicate within the task, not approval of a new business rule. The owner authorises necessary task-related issue requests and replies directly between API and web. Send requests to the receiving repo, link both issues, explain behaviour and contract needs, and let that repo own its plan and implementation. No context repo, middleman or coordinator. Only dependent work waits. Unrelated external messages need authorisation.

**A stack ticket** (`Kind: stack` on its first line) names locally authorised technical decisions rather than business rules. Write one technical ADR per decision in `docs/adr/` (format: `docs/agents/domain.md`); update scaffolding, *Stack and commands*, `CODING_STANDARDS.md` and `.gitignore` as needed. Run every command affected by the change. A decision that does not fit is an owner question on the ticket.

## The API contract

This repo owns the contract, and `stock-flow-web` generates its client from it.

- Publish it in a machine-readable form (for HTTP APIs, OpenAPI), **generated from the code** wherever the framework can, so it cannot drift from what the server does. Here: **`openapi.yaml`** at the repo root, OpenAPI 3.1, written from the Huma operations by `go generate ./...` and checked by CI (ADR 0013). The running API also serves it at `/openapi.yaml`.
- Publish every error code the API can return, in one machine-readable list that `stock-flow-web` checks against. Errors are RFC 9457 Problem Details (ADR 0014). A domain error carries a stable code in Problem Details' `type`, and the list is the `enum` of `ErrorModel.type` in `openapi.yaml`, written from `errorCodes` in `internal/api` (ADR 0019). Declare each status an operation can return in `huma.Operation.Errors`.
- **Changing what an existing request or response means is a breaking change.** Keep published contracts compatible or explicitly version breaking changes. Before merging, name affected callers, the version and compatibility/migration/rollback plan in the PR's *Contract* section. Retain compatibility for the existing pinned web client during adoption.
- Notify web directly of relevant published contract changes using the task-related issue authorisation above. Link the API publication commit, generated contract, affected operations/errors, compatibility and adoption checks. Web adopts a specific API commit and tests integration against it; web does not follow API `main` automatically. A contract change publishes before dependent web adoption. Do not claim adoption until web confirms its pinned API commit and integration result.

## Tests

- **What runs where: `docs/agents/testing.md`.** Locally, the fast layer only; the slow layer runs in CI, and nobody waits for it. Until CI exists, run the slow layer once before opening the pull request.
- Test-first, with the `tdd` skill, at the seam the ticket names.
- **Derive tests from the record's rules**, not from the implementation: every rule gets a test that fails if the rule is removed. A test written from the code inherits the code's blind spots.
- After green, break the rule on purpose once (flip the comparison, delete the guard) and watch a test fail. If none fails, a case is missing.
- Fixtures never return more than the real system would: real empty responses, real paging, real errors.

## Working a local ticket

1. Read the ticket, spec, local record and authorisation. Use a branch from `main`, normally `ticket-<n>`, or an isolated worktree of this repo. The record travels with the checkout.
2. `/implement <ticket>` test-first with `tdd` at the agreed seam. A ticket's authorised seam counts as agreement; a mismatch is a question.
3. Put questions and answers on the API ticket. Address business questions to the owner and implementation/contract questions directly to web when needed. Pause only dependent work. A worker inherits the task's authorisation, never permission to invent rules or merge.
4. Run required checks and `code-review` against the merge-base with `main`, unless the owner specifies another fixed point. Fix findings or list them in the PR.
5. Push the branch, open an implementation PR filling every template section, and link it on the ticket. Report blockers and remaining work. Never merge without explicit authorisation; never edit another repo.

## Commits & pull requests

- Commits, pull requests and ticket comments are written in English, whatever language the session itself uses.
- Branch from `main`; merge through a pull request (a merge request on GitLab).
- Conventional commits, each referencing its ticket (`Refs #12`). `code-review` finds the spec through that reference; without it, the spec half of the review is skipped.
- Fill every section of the pull request template (`.github/pull_request_template.md`, or `.gitlab/merge_request_templates/Default.md`), including when you create the request from the CLI with an explicit body.

## Definition of done

- Record changes pass `bash scripts/check-links.sh`. The fast layer and the linter pass, run in this session; never claim green without a run. The slow layer is CI's, and nobody waits for it; until CI exists, it ran once too (`docs/agents/testing.md`).
- Every product rule the ticket cites has a test that fails without it. Documentation-only workflow changes use link, provenance and workflow checks instead of artificial product tests.
- A contract change is published, and the pull request's *Contract* section names what it breaks.
- `code-review` has run on the branch, and its findings are fixed or listed in the pull request.

## Agent skills

For session boundaries, model/effort choices, stage updates and continuation prompts, read [the session guide](docs/session-guide.md) at task start and before a handoff. Before every `code-review` invocation, read [the local review override](docs/agents/code-review.md); it overrides installed skill defaults for reviewer settings, context, dirty-file coverage and fallback. Apply these rules when `/implement` calls review too. Keep repo overrides here or in `docs/agents/`, not in installed skills; see [skill maintenance](README.md#updating).

### Issue tracker

Issues and specs in this repo on GitHub; the record is local. See `docs/agents/issue-tracker.md`.

### Domain docs

`GLOSSARY.md` and `docs/requirements/` are local. `docs/business-adr/` holds business decisions; `docs/adr/` holds technical decisions. See `docs/agents/domain.md`.
