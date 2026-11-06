-- name: IssueSetupLink :exec
INSERT INTO setup_links (user_id, token_hash, expires_at) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE
SET token_hash = excluded.token_hash, expires_at = excluded.expires_at;

-- name: GetSetupLinkUser :one
SELECT users.name, users.email
FROM setup_links
JOIN users ON users.id = setup_links.user_id
WHERE setup_links.token_hash = sqlc.arg(token_hash) AND setup_links.expires_at > sqlc.arg(at);

-- One statement, so a link opened twice at once works once: the second DELETE waits for
-- the first to commit and then finds no row (users-and-access R27).
-- name: SetPassword :execrows
WITH used AS (
    DELETE FROM setup_links
    WHERE token_hash = sqlc.arg(token_hash) AND expires_at > sqlc.arg(at)
    RETURNING user_id
)
UPDATE users SET password_hash = sqlc.arg(password_hash)::text
FROM used
WHERE users.id = used.user_id;
