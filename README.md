# stock-flow-api

The backend for stock-flow. This repository owns its requirements, glossary, decisions, planning, implementation and generated API contract. Web owns its own work and consumes a specific published API commit.

## Project goal

A practice project for frontend development. Prefer the simplest thing that works. The imported product rules preserve existing behaviour; this ownership migration changes no runtime code, schema or contract.

## Setup

1. Clone this repository. No sibling checkout is required. On Windows, enable native symlink creation (Developer Mode or the required privilege) and clone with `git clone -c core.symlinks=true <repo-url>` so the checked-in agent links work.
2. Install the Go version in `go.mod` and run `go mod download`.
3. Copy `.env.example` to `.env` and fill in local credentials. Run `go run ./cmd/api`.
4. For agent workflows, install Node/npm and Git, then run `bash scripts/install-skills.sh`. Git Bash works on Windows. GitHub work uses an authenticated `gh` CLI.
5. Read [AGENTS.md](AGENTS.md) for exact build, test, format, migration and contract-generation commands.

## Local workflow

Follow [docs/workflow.md](docs/workflow.md): `/grill-with-docs`, local requirements/decisions, record PR, `/to-spec`, `/to-tickets`, `/implement`, tests, `code-review`, implementation PR. Owner authorisation carries through its stated scope. Merges require explicit authorisation.

The record is [requirements](docs/requirements/README.md), [GLOSSARY.md](GLOSSARY.md) and [business ADRs](docs/business-adr/README.md). [Technical ADRs](docs/adr/) retain their existing numbers. [Tracker guidance](docs/agents/issue-tracker.md) explains local specs, ticket readiness and direct API/web issue requests.

`stock-flow-context` is historical reference only. [Migration documentation](docs/migrations/context-ownership.md) records the source commit, file hashes, citation mapping, unresolved questions, forward migration, rollback and remaining dependencies. No context checkout, dispatch service or coordinator is needed.

## API contract

API owns `openapi.yaml`, generated from Huma operations by `go generate ./...` and served at `/openapi.yaml`. The error-code enum is generated here too. CI checks that generated files match sources.

Keep published contracts compatible or explicitly version breaking changes. Notify [stock-flow-web](https://github.com/thawatchaiUdjan/stock-flow-web) directly through linked issues of relevant published changes. Web chooses a specific API commit and tests integration against it; it does not follow API `main` automatically. Adoption remains unconfirmed until web replies with its pinned commit and integration result.

## Email access

Contract version 0.2.0 adds first-time email code setup and self-service password recovery alongside the legacy API during web adoption. API owns the templates. [Configuration, previews and live verification](docs/email/README.md) describe Brevo setup, the four optional environment settings, email-client limits and rollback. Render the actual templates offline with `go run ./cmd/email-preview`; no credentials or email sending are involved.

Legacy-flow retirement remains blocked on confirmed web adoption in [ticket #49](https://github.com/thawatchaiUdjan/stock-flow-api/issues/49). This expansion release does not automatically change the pinned web client or perform a cutover.

## Agent skills

[scripts/install-skills.sh](scripts/install-skills.sh) installs only this repo's skills from [mattpocock/skills](https://github.com/mattpocock/skills), using pinned `skills@1.7.4` and disabled telemetry. Skills live in `.agents/skills/`, with Claude links in `.claude/skills/`; `skills-lock.json` records source paths and hashes.

| Skill | Purpose |
| --- | --- |
| `grill-with-docs` | Planning entry point; calls `grilling` and `domain-modeling` |
| `grilling` | Resolve owner decisions through concrete scenarios |
| `domain-modeling` | Local vocabulary and business decisions; repo namespace rules override its default ADR path |
| `to-spec` | Publish the agreed local spec |
| `to-tickets` | Split the spec into verifiable API slices with actual blocking edges |
| `research` | Primary-source evidence under `docs/research/`; evidence does not confirm a rule |
| `writing-for-agents` | Guidance for maintaining agent-facing documents |
| `implement` | Build an authorised ticket, check it and call `code-review` |
| `tdd` | Test-first at the agreed seam |
| `codebase-design` | Module and interface vocabulary for test seams |
| `code-review` | Independent Standards and Spec reviews |
| `diagnosing-bugs` | Evidence-led diagnosis of hard bugs |
| `setup-matt-pocock-skills` | User-invoked tracker setup; preserve local ownership and namespace policy |

### Updating

Run `bash scripts/install-skills.sh` here. It refreshes only this repository; inspect `git diff skills-lock.json .agents/skills .claude/skills` before committing. Read new skills and scripts before installation. Do not edit installed skill files: put repo overrides in `AGENTS.md` or `docs/agents/`.

For another source, install with `npx skills@1.7.4 add <source> --skill <name> --agent claude-code --agent universal`, then add the name to `OTHER_SKILLS` in the installer. Refresh those skills by name with `npx skills@1.7.4 update -p <name>`. No sibling-wide updater is used.
