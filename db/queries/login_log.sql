-- name: AddLoginLogEntry :exec
INSERT INTO login_log (at, email, user_id, result, ip, user_agent)
VALUES (sqlc.arg(at), sqlc.arg(email), sqlc.narg(user_id), sqlc.arg(result), sqlc.arg(ip), sqlc.arg(user_agent));

-- Newest first by id, which follows the order the entries were written: the cursor
-- (before) is the id of the last entry of the previous page, so entries written while the
-- Owner pages through never shift a page. A NULL filter matches everything.
-- name: ListLoginLog :many
SELECT login_log.id, login_log.at, login_log.email, login_log.user_id, users.name AS user_name,
    login_log.result, login_log.ip, login_log.user_agent
FROM login_log
LEFT JOIN users ON users.id = login_log.user_id
WHERE (sqlc.narg(user_id)::bigint IS NULL OR login_log.user_id = sqlc.narg(user_id))
    AND (sqlc.narg(result)::text IS NULL OR login_log.result = sqlc.narg(result))
    AND (sqlc.narg(before)::bigint IS NULL OR login_log.id < sqlc.narg(before))
ORDER BY login_log.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset)::integer;

-- navigation-and-lists R6, context ADR 0009: count filters before page selection.
-- name: CountLoginLog :one
SELECT count(*) FROM login_log
WHERE (sqlc.narg(user_id)::bigint IS NULL OR login_log.user_id = sqlc.narg(user_id))
    AND (sqlc.narg(result)::text IS NULL OR login_log.result = sqlc.narg(result));

-- users-and-access R55: the newest failures for an email, in any letter case, as far back
-- as a block can still reach. LIMIT is the count that blocks.
-- name: RecentFailedLoginsByEmail :many
SELECT at FROM login_log
WHERE result = 'failed-login' AND lower(email) = lower(sqlc.arg(email)) AND at > sqlc.arg(since)
ORDER BY at DESC
LIMIT 5;

-- users-and-access R56: the newest failures from an address within the last hour.
-- name: RecentFailedLoginsByIP :many
SELECT at FROM login_log
WHERE result = 'failed-login' AND ip = sqlc.arg(ip) AND at > sqlc.arg(since)
ORDER BY at DESC
LIMIT 50;
