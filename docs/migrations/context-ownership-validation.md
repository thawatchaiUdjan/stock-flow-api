# Ownership migration validation

This report covers the documentation/skills migration only. Product code, schema and generated API contract must remain unchanged.

- Baseline: `go build ./...` passed; `go test ./...` passed, 428 tests in eight packages.
- Skill installation: `bash scripts/install-skills.sh` installed all 13 allowlisted skills and dependencies. Existing skill refresh changes add fresh-context instructions to code review and interface exploration; no product behaviour changes.
- Final local checks: `go build ./...`, `go test ./...` (428 tests in eight packages), `go vet -tags slow ./...`, `gofmt -l .` (empty), and `go generate ./...` passed. Generation left SQL output and `openapi.yaml` unchanged.
- Provenance: all 24 committed source blobs, Git blob IDs, SHA-256 hashes and documented link transformations matched. Imported content matched; only requirements README adds local workflow guidance. No product module rule, exclusion, question or glossary definition changed.
- Skills: all 13 installer/lock entries resolve, including required planning dependencies. Claude links are portable relative symlinks. Installer rerun passed after normalising Windows junctions.
- Link validation passed; a deliberately broken relative link made the checker fail, then was removed. `git diff --check` passed. The checker runs in CI too.
- Runtime/database/contract/dependency/deployment files and existing technical ADRs have no diff.
- Independent checkout: a fresh clone under `.git/ownership-independent-checkout` has no context or web sibling, passes documentation link checks, and resolves all 13 Claude skill paths with native symlinks enabled. Windows initially checked symlinks out as text with its default `core.symlinks=false`; setup now documents cloning with `core.symlinks=true` and native symlink privilege.
- Independent `code-review` ran against `2384eabf44f0cebfd6f01feedd44a75d1b78178c...HEAD`, using fresh-context Standards and Spec agents. Both reviewed the final Windows setup addition (`368ebf9`). The Spec reviewer independently checked the 24 source hashes against GitHub blobs. Reports follow.
- Slow tests remain CI's responsibility under `docs/agents/testing.md`; no local slow run is claimed.
- Web pin/adoption and historical security/concurrency questions remain unverified/unresolved as listed in the migration document.

## Standards

No findings. Reviewed workflow, provenance, skill installation, permissions and portability against the repo standards. Imported historical authority is superseded; business/technical namespaces remain distinct; task authorisation and web ownership are preserved. No substantive baseline smells. The final Windows setup addition addresses the observed checkout failure.

## Spec

No findings against API issue #40 and the owner's requested scope, including `368ebf9`. Independent local ownership, preserved record/provenance, separate ADR namespaces, planning skills, direct API/web communication, contract compatibility/adoption, rollback and dependencies are covered. No unrequested product or contract changes. All 24 source hashes and corresponding local import hashes were independently verified, excluding the documented requirements README guidance addition.

Standards: 0 findings; no worst issue. Spec: 0 findings; no worst issue. Owner review and explicit merge authorisation remain pending; CI slow-test results are not claimed here.
