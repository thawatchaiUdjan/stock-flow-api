# 0020: Argon2id for passwords

Status: Accepted
Date: 2026-11-06

## Context

Passwords are stored with a slow hash (`users-and-access` spec). The API runs on Vercel functions (ADR 0008), where memory and time per request are limited. The record sets a minimum of 6 characters (R19) and no maximum.

## Decision

Argon2id from `golang.org/x/crypto/argon2`, with OWASP's minimum settings: 19 MiB of memory, 2 passes, 1 thread, a 16-byte random salt and a 32-byte key. A hash is stored as a PHC string (`$argon2id$v=19$m=19456,t=2,p=1$…`), which carries its own settings, so they can change later without touching the hashes already stored.

`auth.PasswordHasher.Verify` does a hash's work even with nothing to verify against, so a login with an unknown email, or for a User with no password yet, takes as long as a wrong password (R5).

Setup link and session tokens are not passwords: they have 128 random bits, so they are stored as a plain SHA-256.

## Consequences

- A login costs about 19 MiB and tens of milliseconds. Each Vercel instance can do that without tuning.
- Tests use the cheapest settings (`docs/agents/testing.md`), which is why the settings are a value and not constants.
- `golang.org/x/crypto`, already in the module graph, becomes a direct dependency.

## Alternatives considered

- bcrypt: simpler to call, but it refuses passwords over 72 bytes. That would need a maximum length, a rule the record doesn't have.
- scrypt: sound, but OWASP prefers Argon2id, and its Go API offers no standard string format either.
