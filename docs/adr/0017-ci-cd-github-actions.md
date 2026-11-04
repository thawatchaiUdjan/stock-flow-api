# 0017: CI and CD on GitHub Actions

Status: Accepted
Date: 2026-11-04

## Context

Production must only get code that passed CI, migrations must run once per deploy (ADR 0007), and a free Supabase project must see daily activity (ADR 0010).

## Decision

- **`ci.yml`** runs on pull requests and pushes to `main`, with `concurrency` and `cancel-in-progress`, and setup-go's module and build cache. It runs gofmt, `go vet -tags slow ./...` and `go test -tags slow ./...` (both test layers), then `go generate ./...`, and fails if that changed anything: sqlc output and `openapi.yaml` must match their sources.
- **`deploy.yml`** runs when `ci.yml` completes for a push to `main`, and only if it succeeded (`workflow_run`). It runs `go tool goose … up` against `MIGRATION_DATABASE_URL`, then `vercel deploy --prod`. Deploys never overlap and are never cancelled midway.
- **`keepalive.yml`** runs daily on a `schedule`, and by `workflow_dispatch`. It `curl`s the deployed `/health`, at `https://stock-flow-api.vercel.app/health`.
- Vercel's Git deployments are off for this project (`vercel.json`, `git.deploymentEnabled: false`).
- GitHub Actions secrets, set by the owner and never committed: `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`, `MIGRATION_DATABASE_URL`. Branch protection on `main` requires the `ci` check.

## Consequences

- `deploy.yml` and `keepalive.yml` run from `main` only (`workflow_run` and `workflow_dispatch` only see workflows on the default branch), so they first run after this ADR merges.
- If the production domain isn't `stock-flow-api.vercel.app`, change the URL in `keepalive.yml`.
- The slow layer runs on every pipeline, and nobody waits for it (`docs/agents/testing.md`).

## Alternatives considered

- Vercel's Git integration deploying `main`: it would deploy without waiting for CI and without running migrations first.
- golangci-lint: gofmt and `go vet` cover what this project needs today. Add a linter when a recurring mistake calls for one.
