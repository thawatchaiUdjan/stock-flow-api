# Context ownership migration

## Authorisation and acceptance

On 2026-11-11, the requirements owner authorised making this API repo own its requirements and workflow, importing the confirmed context record once and preserving current product behaviour. [API issue #40](https://github.com/thawatchaiUdjan/stock-flow-api/issues/40) captures the request and acceptance criteria. Work is confined to this repository, on a branch, with a reviewable change. Merge is not authorised. [Business ADR 0010](../business-adr/0010-api-owns-record-and-workflow.md) records that scope and replaces historical workflow delegations for API only.

Acceptance: local planning skills and their dependencies are installed through the repo installer; local record/spec/ticket/implementation/review paths work without a context checkout; existing product, security, test and contract obligations remain; direct API/web requests need no extra handoff approval; contract pinning and adoption evidence are explicit; provenance, rollback, checks and unresolved work are reviewable.

## Source and inventory

Source: [stock-flow-context at 4c81bd45ff27cca31bd911e5e429b46c273265d2](https://github.com/thawatchaiUdjan/stock-flow-context/tree/4c81bd45ff27cca31bd911e5e429b46c273265d2). API baseline: `2384eabf44f0cebfd6f01feedd44a75d1b78178c`.

The import reads committed Git blobs, not the context working tree. Its untracked `debug.log` is excluded. [context-import.json](context-import.json) records every original path, destination, Git blob ID, source SHA-256, initial import SHA-256 and link transformation. Initial import hashes describe the one-time import before local ownership guidance was appended; future authorised local changes do not rewrite provenance.

| Source | Local destination | Treatment |
| --- | --- | --- |
| `GLOSSARY.md` | `GLOSSARY.md` | Vocabulary retained verbatim; API now maintains its copy |
| `docs/requirements/` | `docs/requirements/` | All seven modules, IDs, rules, exclusions, statuses and questions retained; business ADR links remapped; README gains local guidance |
| `docs/adr/0001`–`0009` | `docs/business-adr/` | Same numbers and decision text; relative historical links pinned to original source |
| `docs/security-notes.md` | `docs/security-notes.md` | Original findings retained; no claim of fixes |
| `docs/research/` | `docs/research/` | Historical date-control evidence needed by a requirement link; research is not authority |
| `README.md`, `docs/status.md`, `docs/feature-log.md` | `docs/history/context-README.md`, `context-status.md`, `context-feature-log.md` | Historical snapshots; relative links pinned to original context commit |

Context instructions, workflow scripts, skill installer, dispatch/coordinator tools and model-routing policies are not imported as operational dependencies. Their historical references remain in snapshots. The old review queue is neither restored nor treated as reviewed.

Skills were inspected at upstream `mattpocock/skills` commit `4ae2317ddb22b046e26455e4f9c9b3aaaf62212d` and installed with `bash scripts/install-skills.sh`. The repo's pinned `skills@1.7.4`, telemetry setting, allowlist, on-disk checks and lock hashes remain in use. Seven planning/support skills join the six existing skills. Future refreshes are local and reviewed.

## Citation mapping

- `<module> Rn` keeps the same module and rule ID under `docs/requirements/`.
- `context ADR NNNN` maps to `docs/business-adr/NNNN-*.md`, including existing Go, SQL and generated-code comments. Those comments remain untouched.
- Bare `ADR NNNN` inside imported requirements/business ADRs means business ADR NNNN, except explicit API/web technical references.
- Existing bare technical ADR references, including code and `docs/adr/`, keep their old technical meaning. New prose uses `business ADR` or `technical ADR` where ambiguous.
- Historical context specs/PRs and old technical ADR links remain evidence. They neither require context approval nor grant current authority. Carry legacy ticket acceptance criteria into an API issue when resuming work.
- Business 0005 and 0008 retain original text/status as historical evidence. Business 0010 supersedes their workflow locally. All other imported product decisions retain their meaning.

## Forward migration

1. Review this branch's import manifest, record and workflow diff. Confirm no runtime, database or generated contract changes.
2. Run repo checks, link/provenance verification and independent Standards/Spec review. Open the PR with evidence and remaining questions; do not merge without owner authorisation.
3. On explicit owner merge authorisation, merge the record PR; its prepared Accepted status for business ADR 0010 takes effect then. Thereafter plan, record, specify, ticket and implement here using [the local workflow](../workflow.md).
4. Existing closed tickets and context sources remain historical. For open legacy work, move necessary acceptance criteria and unresolved questions into API issues, retain original links and verify present authorisation. Do not bulk reopen completed work.
5. When a task needs web, use direct linked issue requests. Publish compatible or explicitly versioned contracts here first; notify web with the publication commit. Track adoption only from web's explicit pinned commit and integration evidence.

The import is not a sync mechanism. Do not refresh local rules automatically from context or overwrite later local decisions by rerunning an import.

## Rollback

Before merge, close the PR and abandon the branch; main and runtime remain unchanged. After merge, use a reviewed revert PR for the migration commit, rather than resetting history. No data migration, schema rollback, deployment or contract rollback is needed for this docs/skills change.

If local decisions or tickets have accumulated after migration, preserve them and their links first. Reverting old instructions alone could orphan new authority. Decide the replacement workflow with the owner and supersede business ADR 0010 through a new record PR before restoring any external dependency. A revert does not authorise editing web/context, revive old Autopilot grants, or silently return control to a coordinator.

## Remaining dependencies and unresolved questions

- [Users & access open questions](../requirements/users-and-access.md): login throttling/lockout policy and password minimum/maximum/common-password policy remain unanswered. Current behaviour stays unchanged.
- [Security notes](../security-notes.md) retain the 2026-11-08 findings: web headers, direct API access/client-IP verification, SameSite assumptions, unbounded order size and session limits, and checks not performed. They are historical findings, not a fresh audit or proof that they remain unfixed.
- [Historical status](../history/context-status.md) also preserves two reasoned, unreproduced concurrency concerns from API PR #19: Update User/Reset password versus Log out deadlock, and overlapping logins starving password set. Investigate before treating them as verified bugs; nothing here resolves them.
- Web's current pinned API commit and integration status are unverified. Importing old completion claims does not establish adoption or adoption of this workflow. No API contract changes in this task require web implementation or a publication notice.
- GitHub/authenticated `gh` is needed for remote specs, tickets, PRs and direct issue communication. Git, Node/npm, network access and upstream `mattpocock/skills` are needed for skill refreshes. Installed skills are checked in; ordinary record reading needs none of those remote services.
- Go/module downloads and existing Vercel, Supabase/Postgres, CI, deployment credentials and production services remain as before. Slow tests stay in CI. No context checkout or coordination service is required.

## Validation evidence

Baseline `go build ./...` and `go test ./...` passed before editing (428 tests in eight packages). Final checks and independent review evidence are recorded in the PR and [validation report](context-ownership-validation.md). This migration changes documentation, skills and workflow permissions/checks only; no product guard is mutated because no product rule is implemented or changed.
