-- Deleted Products, Users and Orders stay as rows, out of sight (business ADR 0013,
-- products R17, users-and-access R54, orders R20).

-- +goose Up
-- The check pairs mark and author, and holds the state each kind may be Deleted in, so
-- that no query can delete a Product still sold, a User who can log in, the Owner, or an
-- Order that still holds stock.
ALTER TABLE products
    ADD COLUMN deleted_at timestamptz,
    ADD COLUMN deleted_by bigint REFERENCES users (id),
    ADD CONSTRAINT products_deleted_check CHECK (
        (deleted_at IS NULL AND deleted_by IS NULL) OR (deleted_at IS NOT NULL AND deleted_by IS NOT NULL AND status = 'archived')
    );

ALTER TABLE users
    ADD COLUMN deleted_at timestamptz,
    ADD COLUMN deleted_by bigint REFERENCES users (id),
    ADD CONSTRAINT users_deleted_check CHECK (
        (deleted_at IS NULL AND deleted_by IS NULL)
        OR (deleted_at IS NOT NULL AND deleted_by IS NOT NULL AND status = 'inactive' AND role <> 'owner')
    );

ALTER TABLE orders
    ADD COLUMN deleted_at timestamptz,
    ADD COLUMN deleted_by bigint REFERENCES users (id),
    ADD CONSTRAINT orders_deleted_check CHECK (
        (deleted_at IS NULL AND deleted_by IS NULL) OR (deleted_at IS NOT NULL AND deleted_by IS NOT NULL AND status <> 'pending')
    );

-- products R3, users-and-access R3: a Deleted Product's SKU and a Deleted User's email are
-- free again, so uniqueness holds among the rest. The names stay: store.Store tells a SKU
-- clash by products_sku_key.
DROP INDEX products_sku_key;
CREATE UNIQUE INDEX products_sku_key ON products (lower(sku)) WHERE deleted_at IS NULL;
DROP INDEX users_email_key;
CREATE UNIQUE INDEX users_email_key ON users (lower(email)) WHERE deleted_at IS NULL;

-- +goose Down
-- Fails once a Deleted row shares its SKU or email with a live one: that data is the
-- reason to keep this migration.
DROP INDEX users_email_key;
CREATE UNIQUE INDEX users_email_key ON users (lower(email));
DROP INDEX products_sku_key;
CREATE UNIQUE INDEX products_sku_key ON products (lower(sku));
ALTER TABLE orders DROP COLUMN deleted_by, DROP COLUMN deleted_at;
ALTER TABLE users DROP COLUMN deleted_by, DROP COLUMN deleted_at;
ALTER TABLE products DROP COLUMN deleted_by, DROP COLUMN deleted_at;
