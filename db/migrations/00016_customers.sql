-- Customers, their sessions and the sign-up that precedes them (customers R1-R6, R8-R11;
-- business ADR 0014). A Customer is not a User: own table, own sessions, own cookie.
--
-- Additive. The Delivery address (customers R2) arrives with the Storefront's own ticket;
-- Order and Stock movement actor columns arrive with the Online order ticket.

-- +goose Up
CREATE TABLE customers (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (name !~ '^\s*$' AND char_length(name) <= 120), -- customers R2: required, whatever the blank
    email text NOT NULL,
    phone text NOT NULL CHECK (phone ~ '^0[0-9]{8,9}$'), -- customers R4
    password_hash text, -- NULL until the Customer sets one (R7)
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    deleted_by bigint REFERENCES users (id),
    -- customers R16: only an Inactive Customer is Deleted, by a User.
    CONSTRAINT customers_deleted_check CHECK (
        (deleted_at IS NULL AND deleted_by IS NULL)
        OR (deleted_at IS NOT NULL AND deleted_by IS NOT NULL AND status = 'inactive')
    )
);

-- customers R3: an email belongs to one Customer, whatever its letter case; a Deleted
-- Customer's email is free again.
CREATE UNIQUE INDEX customers_email_key ON customers (lower(email)) WHERE deleted_at IS NULL;

-- customers R10: only a hash of the token is stored, as for a User's session.
CREATE TABLE customer_sessions (
    token_hash bytea PRIMARY KEY,
    customer_id bigint NOT NULL REFERENCES customers (id),
    expires_at timestamptz NOT NULL
);

-- customers R5, R8: a sign-up has no Customer yet, so its code, and the grant the code is
-- exchanged for, belong to the email. One row per email: a new code replaces the old one
-- and keeps the failed attempts. email is stored in lower case.
CREATE TABLE signup_credentials (
    email text PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('first-time-code', 'setup-grant')),
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    blocked_until timestamptz NOT NULL DEFAULT 'epoch'
);

-- users-and-access R36: the Customer a Storefront login matches. Customers are never
-- removed, only marked Deleted, so it stays valid.
ALTER TABLE login_log ADD CONSTRAINT login_log_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers (id);

-- A new table closes the Data API in the migration that creates it (ADR 0021).
ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE customer_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE signup_credentials ENABLE ROW LEVEL SECURITY;

-- +goose Down
-- Fails once a Storefront entry names a Customer: login_log entries cannot be removed.
ALTER TABLE login_log DROP CONSTRAINT login_log_customer_id_fkey;
DROP TABLE signup_credentials;
DROP TABLE customer_sessions;
DROP TABLE customers;
