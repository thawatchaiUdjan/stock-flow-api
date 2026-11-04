# 0008: Vercel Go Framework Preset as the runtime

Status: Proposed (Accepted once the first deploy answers from `sin1`)
Date: 2026-11-04

## Context

Hosting must cost nothing (ADR 0016). Render Free sleeps when idle and takes about a minute to wake, which made a demo look broken.

## Decision

The API runs on Vercel's Go Framework Preset (beta), in the Vercel project `stock-flow-api`. It's a normal `net/http` server in `cmd/api/main.go`, one of the entrypoints the preset detects, listening on `PORT`. `vercel.json` sets `"framework": "go"` and `"regions": ["sin1"]` (next to Supabase in Singapore), and turns off Vercel's Git deployments: only `deploy.yml` deploys (ADR 0017).

Fallbacks, in order: if the builder rejects Go 1.27, step back to Go 1.26; if the runtime itself fails, move to Render Free. Whichever happens is recorded here before this ADR is accepted.

## Consequences

- Each instance runs `main()`, so nothing that must happen once (migrations) may run at startup (ADR 0007), and each instance holds its own small pool (ADR 0006).
- Vercel Hobby keeps runtime logs for 1 hour only (`AGENTS.md`).
- No root `api/` directory: Vercel treats one as Serverless Functions. HTTP code lives in `internal/api`.
- The preset is in beta. If it changes behaviour, the fallbacks above apply.

## Alternatives considered

- Render Free: about a minute's cold start after idling.
- Vercel's classic Go runtime (one handler function per file under `api/`): doesn't fit a single chi router serving the whole API.
