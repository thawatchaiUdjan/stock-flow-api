-- name: IssueSetupLink :exec
INSERT INTO setup_links (user_id, token_hash, expires_at) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE
SET token_hash = excluded.token_hash, expires_at = excluded.expires_at;

-- name: GetSetupLinkUser :one
SELECT users.name, users.email
FROM setup_links
JOIN users ON users.id = setup_links.user_id
WHERE setup_links.token_hash = sqlc.arg(token_hash) AND setup_links.expires_at > sqlc.arg(at);

-- One statement, so a link opened twice at once works once (the second DELETE waits for
-- the first to commit and then finds no row, users-and-access R27), and the password and
-- the ended sessions stand or fall together. Setting the Owner's password ends all the
-- Owner's sessions (users-and-access R34, context ADR 0003); no one else's password-set
-- ends any. Returns how many passwords were set: 1, or 0 when the link doesn't work.
-- name: SetPassword :one
WITH used AS (
    DELETE FROM setup_links
    WHERE setup_links.token_hash = sqlc.arg(token_hash) AND setup_links.expires_at > sqlc.arg(at)
    RETURNING user_id
), updated AS (
    UPDATE users SET password_hash = sqlc.arg(password_hash)::text
    FROM used
    WHERE users.id = used.user_id
    RETURNING users.id, users.role
), ended AS (
    DELETE FROM sessions
    WHERE user_id IN (SELECT id FROM updated WHERE role = 'owner')
)
SELECT count(*) FROM updated;
