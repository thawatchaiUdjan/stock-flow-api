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
- Independent Standards/Spec review: pending completion in this branch.
- Slow tests remain CI's responsibility under `docs/agents/testing.md`; no local slow run is claimed.
- Web pin/adoption and historical security/concurrency questions remain unverified/unresolved as listed in the migration document.
