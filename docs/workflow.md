# API workflow

This checkout contains the API record and workflow. No context checkout, dispatch script or coordinator is required. Web owns its own record, plan and implementation. [Tracker guidance](agents/issue-tracker.md) defines local issues and direct cross-repo requests.

## Authorisation

Use the owner's stated task scope and delegation throughout the task. Do not repeat permission questions for already authorised actions. Resolve unanswered business decisions with the requirements owner; an API/web issue request does not authorise new rules. Without delegation, proposals stay Draft and unanswered decisions stay in `Open questions`. Merges always need explicit owner authorisation; this migration is authorised for a reviewable branch and PR only.

Imported workflow ADRs 0005 and 0008 describe past authority, not today's gates. [Business ADR 0010](business-adr/0010-api-owns-record-and-workflow.md) replaces their context coordination and automatic-merge provisions for this API. It preserves confirmed product rules.

## From idea to implementation

1. **Plan:** run `/grill-with-docs <topic>`. Read relevant local requirements, glossary and ADRs first. Discover facts, then resolve owner decisions. Update module rules/questions, vocabulary and business ADRs using [domain guidance](agents/domain.md). Use `research` for primary-source evidence under `docs/research/` when needed. Finish with every decision confirmed within authorisation or explicitly unresolved.
2. **Record PR:** on a branch, submit requirements/decision changes, preserving rule IDs and citing authorisation. A confirmed rule change needs a superseding business ADR. Fill every PR section and run link checks plus required repo checks. Owner review/merge authorisation is required. Completion means a merged record PR; dependent implementation cannot treat a draft as confirmed. Independently confirmed work can continue.
3. **Spec:** `/to-spec <topic>` publishes an API spec linked to the merged record PR, exact rule IDs and business/technical decisions. Agree the highest useful existing test seam. Include observable acceptance criteria, contract compatibility/versioning and exclusions. An existing authorised seam counts as agreement.
4. **Tickets:** `/to-tickets <spec>` proposes verifiable API slices and genuine blocking edges. Approve the breakdown within the owner's stated delegation or obtain the owner's answer. Publish API tickets linked to the API spec and record. The slice ends at the API's observable boundary; request web work directly rather than assigning or editing it. Mark only specified, confirmed, unblocked work `ready-for-agent`.
5. **Implement:** `/implement <ticket>` on a branch from `main`, using `tdd` at the authorised seam. Derive tests from recorded rules, preserve security and transaction guarantees, then deliberately break each implemented rule and prove a test fails. Restore it before the final run. Follow `CODING_STANDARDS.md`. Documentation-only migrations use provenance, links and workflow validation rather than invented runtime tests.
6. **Check and review:** run `go build ./...`, `go test ./...`, `go vet -tags slow ./...`, `gofmt -l .`, `go generate ./...`, and `bash scripts/check-links.sh`. Generated SQL/OpenAPI must match sources. Slow tests run in CI; follow [testing policy](agents/testing.md), including its no-wait rule. Run `code-review` with independent Standards and Spec agents against the merge-base with `main` unless another fixed point is supplied. Fix findings or list them with reasons.
7. **Implementation PR:** fill every PR section, link spec/ticket/record and checks, and name remaining questions or blockers. Commit with Conventional Commits and `Refs #n`, push, open the PR and link it on the ticket. Do not merge without explicit authorisation. CI failures return as local API work; keep its slow-test policy unchanged.

For purely technical work, the authorised spec/ticket can state that no product record change is needed. This migration's owner request is its authorisation; its migration document and API issue capture acceptance criteria without re-grilling settled instructions.

## Contracts and independent progress

API owns code-generated OpenAPI, the error-code list and publication. Keep a published contract compatible or explicitly version the break. Document affected callers, old/new behaviour, compatibility window, migration and rollback in the API PR. Retain the existing contract for pinned clients until a deliberate, authorised retirement; a new web ticket alone is not adoption evidence.

When web work is needed, send a task-related request directly to its repo, link both issues and explain behaviour and contract needs. Let web plan and implement it. Only work depending on its answer waits. After publishing a relevant change, reply with the exact API commit and generated artifact. Web pins a specific API commit and runs integration tests against it. Record web's confirmation of its pin and test result before reporting adoption. Web never tracks API `main` automatically.

For this migration there is no contract change and no required web implementation request. Web's current pin remains unverified. [Migration and rollback](migrations/context-ownership.md) describes remaining dependencies and historical evidence.
