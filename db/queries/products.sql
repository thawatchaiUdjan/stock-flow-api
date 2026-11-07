-- The SKU's letter case is the schema's to judge (products_sku_key): a clash raises a
-- unique violation, which store.Store answers as ErrSKUTaken.
-- name: InsertProduct :one
INSERT INTO products (name, sku, unit, price, minimum_stock)
VALUES (sqlc.arg(name), sqlc.arg(sku), sqlc.arg(unit), sqlc.arg(price), sqlc.arg(minimum_stock))
RETURNING *;

-- name: GetProduct :one
SELECT * FROM products WHERE id = $1;

-- A NULL argument leaves its column as it is. Stock is not here: it changes only through
-- Stock movements (products R6, context ADR 0006).
-- name: ChangeProduct :one
UPDATE products
SET name = coalesce(sqlc.narg(name)::text, name),
    sku = coalesce(sqlc.narg(sku)::text, sku),
    unit = coalesce(sqlc.narg(unit)::text, unit),
    price = coalesce(sqlc.narg(price)::bigint, price),
    minimum_stock = coalesce(sqlc.narg(minimum_stock)::integer, minimum_stock),
    status = coalesce(sqlc.narg(status)::text, status)
WHERE id = sqlc.arg(id)
RETURNING *;

-- A nil list of stock states reaches Postgres as NULL, whose cardinality is NULL, not 0.
-- Sorted by name (products R13), compared without regard to letter case and by bytes, so
-- the order is the same on any database and in the fake. A NULL filter matches everything.
-- strpos rather than LIKE, so that a search for "%" or "_" means those characters
-- (products R14).
-- name: ListProducts :many
SELECT * FROM products
WHERE status = sqlc.arg(status)
    AND (sqlc.narg(search)::text IS NULL
        OR strpos(lower(name), lower(sqlc.narg(search))) > 0
        OR strpos(lower(sku), lower(sqlc.narg(search))) > 0)
    AND (coalesce(cardinality(sqlc.arg(stock_states)::text[]), 0) = 0 OR stock_state = ANY(sqlc.arg(stock_states)::text[]))
ORDER BY lower(name) COLLATE "C", name COLLATE "C", id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountProducts :one
SELECT count(*) FROM products
WHERE status = sqlc.arg(status)
    AND (sqlc.narg(search)::text IS NULL
        OR strpos(lower(name), lower(sqlc.narg(search))) > 0
        OR strpos(lower(sku), lower(sqlc.narg(search))) > 0)
    AND (coalesce(cardinality(sqlc.arg(stock_states)::text[]), 0) = 0 OR stock_state = ANY(sqlc.arg(stock_states)::text[]));
