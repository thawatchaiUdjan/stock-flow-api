-- changes is text cast to jsonb: the pooler runs the simple protocol (ADR 0006), which would send bytes as bytea.
-- name: AddActivity :exec
INSERT INTO activity_log (at, actor_kind, actor_id, actor_name, actor_email, actor_role,
    action, subject_kind, subject_id, subject_name, subject_ref, changes, ip, user_agent)
VALUES (sqlc.arg(at), sqlc.arg(actor_kind), sqlc.narg(actor_id), sqlc.arg(actor_name), sqlc.arg(actor_email),
    sqlc.arg(actor_role), sqlc.arg(action), sqlc.arg(subject_kind), sqlc.arg(subject_id), sqlc.arg(subject_name),
    sqlc.arg(subject_ref), sqlc.arg(changes)::text::jsonb, sqlc.arg(ip), sqlc.arg(user_agent));

-- Newest first by id (activity-log R10). A NULL filter matches everything; the day range
-- is [from_at, to_at) in instants the caller computed from the Shop's days.
-- name: ListActivityLog :many
SELECT * FROM activity_log
WHERE (sqlc.narg(actor_kind)::text IS NULL OR actor_kind = sqlc.narg(actor_kind))
    AND (sqlc.narg(actor_id)::bigint IS NULL OR actor_id = sqlc.narg(actor_id))
    AND (sqlc.narg(subject_kind)::text IS NULL OR subject_kind = sqlc.narg(subject_kind))
    AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action))
    AND (sqlc.narg(from_at)::timestamptz IS NULL OR at >= sqlc.narg(from_at))
    AND (sqlc.narg(to_at)::timestamptz IS NULL OR at < sqlc.narg(to_at))
ORDER BY id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset)::integer;

-- name: CountActivityLog :one
SELECT count(*) FROM activity_log
WHERE (sqlc.narg(actor_kind)::text IS NULL OR actor_kind = sqlc.narg(actor_kind))
    AND (sqlc.narg(actor_id)::bigint IS NULL OR actor_id = sqlc.narg(actor_id))
    AND (sqlc.narg(subject_kind)::text IS NULL OR subject_kind = sqlc.narg(subject_kind))
    AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action))
    AND (sqlc.narg(from_at)::timestamptz IS NULL OR at >= sqlc.narg(from_at))
    AND (sqlc.narg(to_at)::timestamptz IS NULL OR at < sqlc.narg(to_at));
