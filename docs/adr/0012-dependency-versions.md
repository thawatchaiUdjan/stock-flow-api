# 0012: Latest stable versions at scaffolding

Status: Accepted
Date: 2026-11-04

## Context

The stack ticket asked for the latest stable release of everything when it was worked, stepping one back a major only where two don't work together.

## Decision

Resolved on 2026-11-04, all of them working together, so nothing stepped back:

| Component | Version |
| --- | --- |
| Go | 1.27.3 (`go 1.27.0` in `go.mod`, ADR 0001) |
| Huma | v2.40.0 |
| chi | v5.5.0 |
| pgx | v5.12.0 |
| sqlc | v1.31.1 |
| goose | v3.29.0 |
| embedded-postgres | v2.0.1 (Postgres 17.11 in tests) |
| pgtestdb, goosemigrator | `v0.1.2-0.20260807145559-d39d3dc2ee55` (`main`, ADR 0011) |
| godotenv | v1.5.1 (loads `.env` for local runs; already in the graph through goose) |
| Vercel CLI (deploy) | 65.11.0 |
| GitHub Actions | `actions/checkout@v7`, `actions/setup-go@v7` |

## Consequences

- Updates are deliberate `go get` changes, reviewed like code. Nothing floats.
- If the Vercel builder rejects Go 1.27, ADR 0008's fallback (Go 1.26) applies.

## Alternatives considered

- Older, long-established versions: no reason to give up current releases in a new project.
