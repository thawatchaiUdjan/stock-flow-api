-- Products and their stock (products R1-R11, context ADR 0006).

-- +goose Up
CREATE TABLE products (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 120), -- products R2
    -- products R3: letters, digits and hyphens. Case is kept for showing; uniqueness ignores it.
    sku text NOT NULL CHECK (sku ~ '^[A-Za-z0-9-]{1,40}$'),
    unit text NOT NULL CHECK (btrim(unit) <> '' AND char_length(unit) <= 20), -- products R4
    -- Whole satang, from ฿0.00 to ฿9,999,999.99 (products R5, context spec "Money").
    price bigint NOT NULL CHECK (price BETWEEN 0 AND 999999999),
    minimum_stock integer NOT NULL DEFAULT 0 CHECK (minimum_stock >= 0), -- products R8
    -- products R6, R7. Nothing sets it yet: the Stock movements feature changes it only
    -- together with the movement that explains the change (context ADR 0006).
    stock integer NOT NULL DEFAULT 0 CHECK (stock >= 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')), -- products R11
    -- products R9, worked out by the database so that no query can disagree with another
    -- about it, and the List can filter on it.
    stock_state text GENERATED ALWAYS AS (
        CASE WHEN stock = 0 THEN 'out-of-stock' WHEN stock <= minimum_stock THEN 'low' ELSE 'in-stock' END
    ) STORED NOT NULL
);

-- products R3: a SKU belongs to one Product, whatever its letter case.
CREATE UNIQUE INDEX products_sku_key ON products (lower(sku));

-- A new table closes the Data API in the migration that creates it (ADR 0021).
ALTER TABLE products ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE products;
