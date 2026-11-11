# 0024: Protect email codes and serialize password credential transitions

Status: Accepted
Date: 2026-11-11
Affects: api
Authorised by: Business ADR 0011 and delegated technical implementation scope in API spec #46, tickets #47 and #48.

## Context

A six-digit code has too little entropy for an ordinary unkeyed hash to protect it from someone reading the database. Multiple Vercel instances must also share attempt limits and single-use state rather than trusting process-local memory.

## Decision

Store First-time code digests as HMAC-SHA-256 with `EMAIL_CODE_SECRET`, a separately configured secret of at least 32 random bytes shared across instances. Bind the digest to the code purpose and case-normalized email. Keep the secret outside the database and repository. Random setup grants and Reset links use the existing cryptographically random token and SHA-256 digest functions.

Persist one current email credential per User, including purpose, expiry and failed-code/block state. Serialize transitions by locking the legacy Setup link, when present, before the User, matching existing password/status operations. Validate before password hashing, then recheck under lock on completion; consume credentials, replace the password, revoke sessions and remove other setup/recovery credentials in one transaction.

Use a rolling-hour event ledger with hashed email/IP identities for shared request limits. Acquire consistent email-then-IP advisory locks around allowance checks/reservation and prune expired events on requests. Suppressed/failed requests cannot reveal eligibility through response payloads. No raw email/IP is stored in this ledger.

## Consequences

- A database-only reader cannot enumerate usable codes without the independent secret. A reader with both database and deployment secrets is outside that protection.
- Code-secret rotation invalidates existing code verification; deploy consistently and let Users request fresh codes. Random grants/Reset links do not depend on this secret.
- Real transaction/concurrency guarantees are covered by slow PostgreSQL tests in CI; shared port contracts keep the fast fake honest.
- Provider delivery is awaited with a bounded timeout; no unawaited serverless background task is responsible for a password email. Provider acceptance is not proof of inbox delivery.

## Alternatives considered

- Plain SHA-256 for codes: all one million values can be enumerated offline.
- In-memory limits or credentials: lost on cold starts and bypassed across instances.
- Holding a database transaction open during external email delivery: couples database locks to network latency; committed credentials may instead be replaced through a limited retry if delivery fails.
