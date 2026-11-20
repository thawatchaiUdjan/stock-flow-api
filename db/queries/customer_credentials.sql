-- Every credential change holds the Customer first, as it does for a User.
-- name: LockCustomer :one
SELECT * FROM customers WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- Resending must not reset failed attempts or a current block (customers R8).
-- name: IssueCustomerCredential :exec
INSERT INTO customer_credentials (customer_id, kind, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (customer_id) DO UPDATE
SET kind = excluded.kind, token_hash = excluded.token_hash, expires_at = excluded.expires_at;

-- name: GetCustomerCredential :one
SELECT * FROM customer_credentials WHERE customer_id = $1;

-- name: SetCustomerCodeAttempts :exec
UPDATE customer_credentials SET failed_attempts = $2, blocked_until = $3 WHERE customer_id = $1;

-- name: DeleteCustomerCredential :exec
DELETE FROM customer_credentials WHERE customer_id = $1;

-- name: GetCustomerCredentialUser :one
SELECT customers.* FROM customer_credentials
JOIN customers ON customers.id = customer_credentials.customer_id
WHERE customer_credentials.kind = sqlc.arg(kind)
  AND customer_credentials.token_hash = sqlc.arg(token_hash)
  AND customer_credentials.expires_at > sqlc.arg(at)
  AND customers.status = 'active'
  AND customers.deleted_at IS NULL
  AND ((customer_credentials.kind = 'setup-grant' AND customers.password_hash IS NULL)
    OR (customer_credentials.kind = 'reset-link' AND customers.password_hash IS NOT NULL));

-- name: SetCustomerPassword :one
UPDATE customers SET password_hash = $2 WHERE id = $1 RETURNING *;

-- name: DeleteCustomerSessions :exec
DELETE FROM customer_sessions WHERE customer_id = $1;

-- name: UpdateCustomerProfile :one
UPDATE customers SET name = sqlc.arg(name), phone = sqlc.arg(phone),
    address_recipient = sqlc.narg(address_recipient), address_phone = sqlc.narg(address_phone),
    address_line = sqlc.narg(address_line), address_subdistrict = sqlc.narg(address_subdistrict),
    address_district = sqlc.narg(address_district), address_province = sqlc.narg(address_province),
    address_postcode = sqlc.narg(address_postcode)
WHERE id = sqlc.arg(id) RETURNING *;
