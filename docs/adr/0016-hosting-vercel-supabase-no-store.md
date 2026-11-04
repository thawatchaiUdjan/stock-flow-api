# 0016: Vercel Hobby and Supabase Free, and every response no-store

Status: Accepted
Date: 2026-11-04

A shared decision, made the same way in `stock-flow-web`'s stack ticket.

## Context

This is a practice project: hosting must cost nothing and need no card, and the demo must answer quickly from Southeast Asia.

## Decision

- Everything runs on Vercel Hobby plus Supabase Free: $0, no card.
- Two Vercel projects, both in `sin1` (Singapore). The web project deploys production from `main`, with a preview per pull request. The api project uses the Go Framework Preset (ADR 0008) and is deployed only by GitHub Actions (ADR 0017).
- Supabase Free, Singapore (`ap-southeast-1`), created through Vercel's Supabase integration and connected to the api project only, Production environment only. The web project holds no Supabase variables.
- **Every API response sends `Cache-Control: no-store`.** The web origin reaches this API through an external rewrite (ADR 0015), and Vercel caches rewritten responses by the upstream `Cache-Control`. The `noStore` middleware in `internal/api` sets it on every route, including errors, unknown paths and the served contract.

## Consequences

- Free-tier limits apply: function duration and invocations on Vercel, a 500 MB database, and pausing after a week idle on Supabase (ADR 0010).
- Only the Production environment has database variables, so no Vercel preview deployment of the api can reach the database. The api has none anyway (ADR 0008).

## Alternatives considered

- Render Free for the api: about a minute's cold start (ADR 0008).
- Allowing caching of some reads: nothing is cacheable yet, and session-dependent responses must never be shared.
