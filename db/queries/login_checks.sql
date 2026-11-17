-- name: PruneLoginChecks :exec
DELETE FROM login_checks WHERE checked_at <= $1;

-- The oldest counted check says when the next one frees up; with none, the argument.
-- name: LoginCheckUsage :one
SELECT count(*) AS check_count,
       coalesce(min(checked_at), sqlc.arg(at)::timestamptz)::timestamptz AS oldest_at
FROM login_checks
WHERE ip_hash = sqlc.arg(ip_hash) AND checked_at > sqlc.arg(hour_start);

-- name: RecordLoginCheck :exec
INSERT INTO login_checks (ip_hash, checked_at) VALUES ($1, $2);
