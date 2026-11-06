-- Starts a session only for the User as they were when the password was checked: Active,
-- with that password. A password check takes long enough for the Owner to set them
-- Inactive or reset them, or for the Owner's own password to be set, in the meantime, and
-- then nothing is inserted (users-and-access R16, R31, R34). FOR SHARE makes it wait for a
-- change under way, and makes store.Store's UpdateUser, ResetPassword and SetPassword wait
-- for this insert, so their later DELETE of the User's sessions finds this one.
-- name: CreateSession :execrows
INSERT INTO sessions (token_hash, user_id, expires_at)
SELECT sqlc.arg(token_hash), users.id, sqlc.arg(expires_at)
FROM users
WHERE users.id = sqlc.arg(user_id)
    AND users.status = 'active'
    AND users.password_hash IS NOT DISTINCT FROM sqlc.narg(password_hash)
FOR SHARE OF users;

-- name: GetSession :one
SELECT sessions.user_id, users.name, users.email, users.role, sessions.expires_at
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = $1;

-- The number of rows says whether this call ended the session or another got there first.
-- name: DeleteSession :execrows
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;
