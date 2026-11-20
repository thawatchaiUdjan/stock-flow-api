-- name: GetCustomerByEmail :one
SELECT * FROM customers WHERE lower(email) = lower($1) AND deleted_at IS NULL;

-- Starts a session only for the Customer as they were when the password was checked:
-- Active, with that password (customers R9, R15), as CreateSession does for a User.
-- name: CreateCustomerSession :execrows
INSERT INTO customer_sessions (token_hash, customer_id, expires_at)
SELECT sqlc.arg(token_hash), customers.id, sqlc.arg(expires_at)
FROM customers
WHERE customers.id = sqlc.arg(customer_id)
    AND customers.status = 'active'
    AND customers.deleted_at IS NULL
    AND customers.password_hash IS NOT DISTINCT FROM sqlc.narg(password_hash)
FOR SHARE OF customers;

-- name: GetCustomerSession :one
SELECT customer_sessions.customer_id, customers.name, customers.email, customers.phone, customer_sessions.expires_at,
    customers.address_recipient, customers.address_phone, customers.address_line, customers.address_subdistrict,
    customers.address_district, customers.address_province, customers.address_postcode
FROM customer_sessions
JOIN customers ON customers.id = customer_sessions.customer_id
WHERE customer_sessions.token_hash = $1;

-- The number of rows says whether this call ended the session or another got there first.
-- name: DeleteCustomerSession :execrows
DELETE FROM customer_sessions WHERE token_hash = $1;

-- Resending must not reset failed attempts or a current block (customers R8).
-- name: IssueSignupCredential :exec
INSERT INTO signup_credentials (email, kind, token_hash, expires_at)
VALUES (lower(sqlc.arg(email)), sqlc.arg(kind), sqlc.arg(token_hash), sqlc.arg(expires_at))
ON CONFLICT (email) DO UPDATE
SET kind = excluded.kind, token_hash = excluded.token_hash, expires_at = excluded.expires_at;

-- name: GetSignupCredential :one
SELECT * FROM signup_credentials WHERE email = lower($1);

-- name: GetSignupGrant :one
SELECT * FROM signup_credentials
WHERE kind = 'setup-grant' AND token_hash = sqlc.arg(token_hash) AND expires_at > sqlc.arg(at);

-- name: SetSignupCodeAttempts :exec
UPDATE signup_credentials SET failed_attempts = sqlc.arg(failed_attempts), blocked_until = sqlc.arg(blocked_until) WHERE email = lower(sqlc.arg(email));

-- name: DeleteSignupCredential :exec
DELETE FROM signup_credentials WHERE email = lower($1);

-- name: CreateCustomer :one
INSERT INTO customers (name, email, phone, password_hash, created_at)
VALUES (sqlc.arg(name), sqlc.arg(email), sqlc.arg(phone), sqlc.arg(password_hash), sqlc.arg(created_at))
ON CONFLICT (lower(email)) WHERE deleted_at IS NULL DO NOTHING
RETURNING *;
