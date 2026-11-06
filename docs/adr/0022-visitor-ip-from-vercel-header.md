# 0022: The visitor's IP address comes from X-Vercel-Forwarded-For

Status: Proposed
Date: 2026-11-06

## Context

The Login log stores the visitor's IP address, "not Vercel's" (`users-and-access` R36, and the spec's *Login log* section). A request reaches this API twice through Vercel: the browser calls the web project, whose `vercel.json` rewrites `/api/:path*` to this API's production URL (ADR 0015), a second Vercel project. Vercel's request-header documentation says `X-Forwarded-For` and `X-Real-Ip` carry the client's IP, that Vercel overwrites a client-supplied `X-Forwarded-For` rather than appending to it, and that `X-Vercel-Forwarded-For` is identical, except that `X-Forwarded-For` "could be overwritten if you're using a proxy on top of Vercel". Which of these still holds the visitor's address on the second hop is not documented, and no deployment was available to try it when this was written.

## Decision

- `internal/api` takes the first valid IP address from `X-Vercel-Forwarded-For`, then `X-Forwarded-For`, then `X-Real-Ip`, and last the connection's remote address (what a local run sees). A header that is not an IP address is passed over, so nothing but an address is ever stored.
- A visitor can only choose the stored address if Vercel lets their own header through. Vercel documents that it sets these headers itself and discards the client's.
- **Proposed until it is checked on a deployment**, because the choice and the order above rest on documentation, not on an observed request. The check: log in through the web origin (not the API's own URL) from a known address, then read the newest entry of `GET /login-log` and compare its `ip`. If it shows a Vercel address instead, this ADR is superseded by one naming the header that carries the visitor's.

## Consequences

- If the check fails, every entry written until it is fixed holds the wrong address. Entries cannot be edited (`users-and-access` R39), so those stay wrong.
- The remote-address fallback is never right on Vercel; it is there so a local run and the tests record something.

## Alternatives considered

- `X-Forwarded-For` only: the common choice, but Vercel's own documentation says a proxy in front of Vercel can overwrite it, and the rewrite may count as one.
- A header the web project adds to the request it rewrites: not known to be possible in `vercel.json`, and a copy the visitor sets would be spoofable unless the API could tell the web project's calls apart.
