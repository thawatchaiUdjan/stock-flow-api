-- users-and-access R3: emails compare without regard to letter case.
-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(sqlc.arg(email));

-- name: GetOwner :one
SELECT * FROM users WHERE role = 'owner';

-- Does nothing when an Owner exists, so concurrent installs can't fail or make a second
-- one: the loser waits for the winner's commit, then skips (users-and-access R9).
-- name: CreateOwner :exec
INSERT INTO users (name, email, role) VALUES ($1, $2, 'owner') ON CONFLICT DO NOTHING;

-- name: ListUsers :many
SELECT id, name, email, role, status, (password_hash IS NOT NULL)::boolean AS password_set
FROM users
ORDER BY id;

-- The User and their Setup link are one statement, so there is never a User nobody can
-- give a password. An email already in use (compared without regard to letter case,
-- users-and-access R3) inserts nothing, issues no link and returns no row: pgx.ErrNoRows,
-- which is store.ErrNotFound.
-- name: CreateUser :one
WITH new_user AS (
    INSERT INTO users (name, email, role)
    VALUES (sqlc.arg(name), sqlc.arg(email), sqlc.arg(role))
    ON CONFLICT (lower(email)) DO NOTHING
    RETURNING id, name, email, role, status
), link AS (
    INSERT INTO setup_links (user_id, token_hash, expires_at)
    SELECT id, sqlc.arg(token_hash), sqlc.arg(expires_at) FROM new_user
)
SELECT id, name, email, role, status FROM new_user;

-- Holds the User's row until the transaction ends (store.Store says why).
-- name: LockUser :one
SELECT role, status FROM users WHERE id = $1 FOR UPDATE;

-- An empty argument leaves its column as it is. The email is not here: it never changes
-- (users-and-access R15).
-- name: ChangeUser :one
UPDATE users
SET name = coalesce(nullif(sqlc.arg(name)::text, ''), name),
    role = coalesce(nullif(sqlc.arg(role)::text, ''), role),
    status = coalesce(nullif(sqlc.arg(status)::text, ''), status)
WHERE id = sqlc.arg(id)
RETURNING id, name, email, role, status, (password_hash IS NOT NULL)::boolean AS password_set;

-- name: SetUserPassword :one
UPDATE users SET password_hash = sqlc.arg(password_hash)::text
WHERE id = sqlc.arg(id)
RETURNING role;

-- name: ClearPassword :exec
UPDATE users SET password_hash = NULL WHERE id = $1;
