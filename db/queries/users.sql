-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(sqlc.arg(email));

-- name: GetOwner :one
SELECT * FROM users WHERE role = 'owner';

-- Does nothing when an Owner exists, so concurrent installs can't fail or make a second
-- one: the loser waits for the winner's commit, then skips (users-and-access R9).
-- name: CreateOwner :exec
INSERT INTO users (name, email, role) VALUES ($1, $2, 'owner') ON CONFLICT DO NOTHING;
