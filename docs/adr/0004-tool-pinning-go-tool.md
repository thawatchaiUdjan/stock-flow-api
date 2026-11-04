# 0004: sqlc and goose pinned with go.mod's tool directive

Status: Accepted
Date: 2026-11-04

## Context

Generating queries (sqlc) and migrating (goose) need the same tool versions on every machine and in CI. Globally installed tools drift apart.

## Decision

Both are declared with the `tool` directive in `go.mod` and run as `go tool sqlc` and `go tool goose`. Nobody installs them globally.

## Consequences

- `go.mod` and `go.sum` pin their versions; updating one is a reviewed `go get -tool` change.
- Their dependencies (sqlc's pull in many database drivers) appear in `go.mod` as indirect requirements, but none are linked into the API binary.
- The first `go tool sqlc` on a machine compiles it, which takes a minute; later runs use the build cache.

## Alternatives considered

- `go install` per developer, or a `tools.go` file with blank imports: the former doesn't pin versions, and the latter is the workaround the `tool` directive replaced.
