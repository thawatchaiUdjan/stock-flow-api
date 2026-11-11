-- name: CreateEmailUser :one
INSERT INTO users (name, email, role)
VALUES ($1, $2, $3)
ON CONFLICT (lower(email)) DO NOTHING
RETURNING *;

-- Every credential transition holds the User, after its legacy Setup link, so old
-- and new password/status changes share the same lock order.
-- name: LockEmailAuthUser :one
SELECT * FROM users WHERE id = $1 FOR UPDATE;

-- Resending must not reset failed attempts or a current block (users-and-access R41).
-- name: IssueEmailCredential :exec
INSERT INTO email_credentials (user_id, kind, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE
SET kind = excluded.kind, token_hash = excluded.token_hash, expires_at = excluded.expires_at;

-- name: GetEmailCredential :one
SELECT * FROM email_credentials WHERE user_id = $1;

-- name: SetEmailCodeAttempts :exec
UPDATE email_credentials
SET failed_attempts = $2, blocked_until = $3
WHERE user_id = $1;

-- name: DeleteEmailCredential :exec
DELETE FROM email_credentials WHERE user_id = $1;

-- name: GetEmailCredentialUser :one
SELECT users.* FROM email_credentials
JOIN users ON users.id = email_credentials.user_id
WHERE email_credentials.kind = sqlc.arg(kind)
  AND email_credentials.token_hash = sqlc.arg(token_hash)
  AND email_credentials.expires_at > sqlc.arg(at)
  AND users.status = 'active'
  AND ((email_credentials.kind = 'setup-grant' AND users.password_hash IS NULL AND users.role <> 'owner')
    OR (email_credentials.kind = 'reset-link' AND users.password_hash IS NOT NULL));

-- Namespace prefixes and one consistent email-then-IP lock order avoid deadlocks.
-- name: LockEmailRequestKey :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text, 0));

-- name: EmailRequestUsage :one
SELECT
    (SELECT count(*) FROM email_request_events AS hourly_email
     WHERE hourly_email.email_hash = sqlc.arg(email_hash) AND hourly_email.requested_at > sqlc.arg(hour_start)) AS email_count,
    (SELECT count(*) FROM email_request_events AS cooldown_email
     WHERE cooldown_email.email_hash = sqlc.arg(email_hash) AND cooldown_email.requested_at > sqlc.arg(minute_start)) AS recent_count,
    (SELECT count(*) FROM email_request_events AS hourly_ip
     WHERE hourly_ip.ip_hash = sqlc.arg(ip_hash) AND hourly_ip.requested_at > sqlc.arg(hour_start)) AS ip_count;

-- name: RecordEmailRequest :exec
INSERT INTO email_request_events (email_hash, ip_hash, requested_at) VALUES ($1, $2, $3);

-- name: PruneEmailRequests :exec
DELETE FROM email_request_events WHERE requested_at <= $1;
