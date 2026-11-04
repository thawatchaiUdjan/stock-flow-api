# 0001: Go 1.27 for the API

Status: Accepted
Date: 2026-11-04

## Context

stock-flow is a practice project for frontend development (`stock-flow-context` README, *Project goal*). The backend exists to serve the frontend, so it should be the simplest thing that works, in a language its owner can maintain without learning it first.

## Decision

The API is written in Go 1.27: `go 1.27.0` (the minimum) and `toolchain go1.27.3` (what it's built with) in `go.mod`. The owner already works in Go.

## Consequences

- One static binary, a standard library HTTP server, and fast tests.
- Vercel reads the Go version from the `go` directive (ADR 0008). The directive says `1.27.0`, the lowest 1.27 release, so a builder on any 1.27 patch accepts it.

## Alternatives considered

- TypeScript (Node or Bun), the same language as `stock-flow-web`: rejected because the owner already knows Go, and code sharing with the web app isn't needed once the client is generated from the contract (ADR 0013).
