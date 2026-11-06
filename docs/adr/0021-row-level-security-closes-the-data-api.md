# 0021: Row Level Security with no policies, to close Supabase's Data API

Status: Accepted
Date: 2026-11-06

## Context

Supabase serves every table in the `public` schema through its Data API, as the roles `anon` and `authenticated`, which it grants everything there. Without Row Level Security, anyone holding the project's anon key could read `users`, password hashes included. The anon key isn't published, but nothing in the system needs that door open: the web app talks only to this API (ADR 0015).

ADR 0015 keeps authorization out of Row Level Security. The requirements owner decided on 2026-11-06 to enable it anyway, only to close that door.

## Decision

- Every table of ours has Row Level Security enabled with **no policies**, in the migration that creates it. No policy means every role that doesn't own the table sees no rows and writes none.
- It is **not** `FORCE`d, because the API reaches the tables as their owner.
- **Authorization stays in Go.** This adds no rule to the database, and ADR 0015 stands: a policy here would be a second place where access is decided.
- `POSTGRES_URL`'s role must own the tables or have `BYPASSRLS`. `.env.example` records both `POSTGRES_URL` and `MIGRATION_DATABASE_URL` as the Supabase role `postgres` (`postgres.<project-ref>` on the poolers), and whoever runs the migrations owns what they create, so the two must stay the same role. Not yet checked against the deployed values, which no repo holds: confirm it with the pooler check of ADR 0006, by comparing the user in both URLs.

## Consequences

- If `POSTGRES_URL` ever connects as another role, reads come back empty: every login fails as a wrong password would, with nothing logged. Writes are refused out loud, so `cmd/install` fails at once, which is the first thing to try. A change of role needs the tables' owner changed with it.
- `goose_db_version` stays open. It holds only migration numbers, and goose owns its creation.
- The slow layer runs as a role that owns the tables and is neither superuser nor `BYPASSRLS`, and asserts that it is, so the whole slow layer proves the API works through this. Another slow test fails if a table is added without Row Level Security.

## Alternatives considered

- Revoking `anon` and `authenticated`'s grants: Supabase's default privileges grant them again on each new table, so each migration would have to remember, and nothing shows it at a glance in the dashboard.
- Turning the Data API off in the Supabase dashboard: a setting outside the repo, which no test sees and a new project wouldn't have. Worth doing as well, but not instead.
- Moving the tables out of `public`: closes the door too, but costs a schema on every query for no further gain.
