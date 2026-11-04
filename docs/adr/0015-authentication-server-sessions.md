# 0015: Server-side sessions in a first-party cookie

Status: Accepted
Date: 2026-11-04

A shared decision, made the same way in `stock-flow-web`'s stack ticket. No code implements it yet: the first feature with users does.

## Context

The system has two kinds of user, owner and staff, using the web app. Credentials must not be readable by page scripts, and the browser should talk to a single origin.

## Decision

- Authentication uses server-side sessions owned by this API. The session ID travels in a cookie that is `HttpOnly`, `Secure` and `SameSite=Lax`. No token in browser storage, no Supabase Auth.
- Two roles: owner and staff. Authorization is checked in Go, not with Supabase Row Level Security.
- The browser talks only to the web origin: the web project's `vercel.json` rewrites `/api/:path*` to this API's Vercel production URL (a literal URL, since rewrites don't expand variables). The cookie is first-party and no CORS is needed. This API therefore sends no CORS headers.

## Consequences

- Sessions need a table and a migration, added with the first feature that logs in.
- Rewritten responses can be cached by Vercel, which is why every response is `no-store` (ADR 0016).
- If `supabase-postgres-best-practices` is ever installed as an agent skill, `AGENTS.md` must say authorization lives in Go, not in RLS.

## Alternatives considered

- JWTs in browser storage: exposed to any script on the page.
- Supabase Auth: a second user store beside the API's own roles, with authorization pulled towards RLS.
