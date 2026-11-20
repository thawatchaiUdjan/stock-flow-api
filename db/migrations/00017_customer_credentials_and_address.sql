-- A Customer's password setup and recovery, and their saved Delivery address (customers
-- R2, R7, R8, R12, R13; storefront R7). Additive.

-- +goose Up
-- One current credential per Customer, as email_credentials is for a User: a Verification
-- code, the grant it is exchanged for, or a Reset link. A new one replaces the old.
CREATE TABLE customer_credentials (
    customer_id bigint PRIMARY KEY REFERENCES customers (id),
    kind text NOT NULL CHECK (kind IN ('first-time-code', 'setup-grant', 'reset-link')),
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    blocked_until timestamptz NOT NULL DEFAULT 'epoch'
);
ALTER TABLE customer_credentials ENABLE ROW LEVEL SECURITY;

-- storefront R7: none, or all seven. Which of the 77 provinces is checked by the API, which
-- publishes the list; the database holds the shape.
ALTER TABLE customers
    ADD COLUMN address_recipient text,
    ADD COLUMN address_phone text,
    ADD COLUMN address_line text,
    ADD COLUMN address_subdistrict text,
    ADD COLUMN address_district text,
    ADD COLUMN address_province text,
    ADD COLUMN address_postcode text,
    ADD CONSTRAINT customers_address_check CHECK (
        (address_recipient IS NULL AND address_phone IS NULL AND address_line IS NULL AND address_subdistrict IS NULL
            AND address_district IS NULL AND address_province IS NULL AND address_postcode IS NULL)
        OR (address_recipient !~ '^\s*$' AND char_length(address_recipient) <= 120
            AND address_phone ~ '^0[0-9]{8,9}$'
            AND address_line !~ '^\s*$' AND char_length(address_line) <= 200
            AND address_subdistrict !~ '^\s*$' AND char_length(address_subdistrict) <= 80
            AND address_district !~ '^\s*$' AND char_length(address_district) <= 80
            AND address_province !~ '^\s*$'
            AND address_postcode ~ '^[0-9]{5}$')
    );

-- +goose Down
ALTER TABLE customers
    DROP CONSTRAINT customers_address_check,
    DROP COLUMN address_postcode, DROP COLUMN address_province, DROP COLUMN address_district,
    DROP COLUMN address_subdistrict, DROP COLUMN address_line, DROP COLUMN address_phone, DROP COLUMN address_recipient;
DROP TABLE customer_credentials;
