# 0011: Go testing, with embedded-postgres and pgtestdb for the slow layer

Status: Accepted
Date: 2026-11-04

## Context

`docs/agents/testing.md` splits tests into a fast layer (fakes, runs locally) and a slow layer (a real database, runs in CI). The slow layer must use real Postgres, run on Windows and in CI, and stay cheap per test. The owner has seen testcontainers make suites slow over time.

## Decision

- Tests use the standard `testing` package.
- **The layers are split by a build tag.** Slow test files start with `//go:build slow`. `go test ./...` runs the fast layer; `go test -tags slow ./...` runs both.
- **Slow tests** get a real Postgres from `internal/testdb`. `testdb.Main`, called from the package's `TestMain`, starts one embedded-postgres v2 server (Postgres 17, the major version Supabase runs) on a free port, with `fsync`, `synchronous_commit` and `full_page_writes` off. `testdb.URL(t)` uses pgtestdb with its goose migrator: the migrations build a template database once, and each test gets a fresh clone, dropped after it passes.
- pgtestdb's goose-migrator fix is only on `main`, so both pgtestdb modules are pinned to the pseudo-version `v0.1.2-0.20260807145559-d39d3dc2ee55`.

## Consequences

- No Docker. The first run downloads the Postgres binaries into a user cache; later starts take a few seconds.
- Per-test isolation costs one `CREATE DATABASE … TEMPLATE`, not a migration run.
- **This departs from `docs/agents/testing.md`** ("One shared test database, not one container per test package"): each slow package's `TestMain` starts its own embedded server, as the stack ticket specified. With one slow package today the cost is a single start. If the slow layer grows to several packages and the starts add up, share one server: pgtestdb already shares templates across packages pointed at one server.
- `go vet` and editors need `-tags slow` to see slow files (`ci.yml` passes it).

## Alternatives considered

- testcontainers-go: needs Docker, and the owner has seen it slow suites down over time.
- `testing.Short()` instead of a build tag: `go test ./...` would then run the slow layer by default, which is the opposite of what `testing.md` wants locally.
