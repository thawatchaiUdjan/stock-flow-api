-- name: IssueSetupLink :exec
INSERT INTO setup_links (user_id, token_hash, expires_at) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE
SET token_hash = excluded.token_hash, expires_at = excluded.expires_at;

-- name: DeleteSetupLink :exec
DELETE FROM setup_links WHERE user_id = $1;

-- Holds the User's Setup link, if any, until the transaction ends (store.Store says why).
-- name: LockSetupLink :exec
SELECT user_id FROM setup_links WHERE user_id = $1 FOR UPDATE;

-- name: GetSetupLinkUser :one
SELECT users.name, users.email
FROM setup_links
JOIN users ON users.id = setup_links.user_id
WHERE setup_links.token_hash = sqlc.arg(token_hash) AND setup_links.expires_at > sqlc.arg(at);

-- One DELETE, so a link opened twice at once works once: the second waits for the first to
-- commit and then finds no row (users-and-access R27).
-- name: UseSetupLink :one
DELETE FROM setup_links
WHERE token_hash = sqlc.arg(token_hash) AND expires_at > sqlc.arg(at)
RETURNING user_id;
