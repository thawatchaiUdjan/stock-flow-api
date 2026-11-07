-- Holds each Product until the transaction ends, so movements for one Product run one
-- after the other, each on the stock the last one left (stock-movements R7). Rows are
-- locked in the order of the ORDER BY: always by id, so two transactions naming the same
-- Products in a different order wait for each other instead of deadlocking.
-- name: LockProducts :many
SELECT id, stock, status, price FROM products
WHERE id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY id
FOR UPDATE;

-- Only with the movement that explains it (context ADR 0006).
-- name: SetProductStock :exec
UPDATE products SET stock = sqlc.arg(stock) WHERE id = sqlc.arg(id);

-- name: InsertStockMovement :one
INSERT INTO stock_movements (kind, product_id, change, stock_after, at, user_id, note, order_id)
VALUES (sqlc.arg(kind), sqlc.arg(product_id), sqlc.arg(change), sqlc.arg(stock_after), sqlc.arg(at),
    sqlc.arg(user_id), sqlc.arg(note), sqlc.narg(order_id))
RETURNING id;

-- The columns of ListStockMovements, so a row of one converts to a row of the other.
-- name: GetStockMovement :one
SELECT stock_movements.id, stock_movements.kind, stock_movements.product_id,
    products.name AS product_name, products.sku AS product_sku,
    stock_movements.change, stock_movements.stock_after, stock_movements.at,
    stock_movements.user_id, users.name AS user_name, stock_movements.note,
    stock_movements.order_id, orders.number AS order_number
FROM stock_movements
JOIN products ON products.id = stock_movements.product_id
JOIN users ON users.id = stock_movements.user_id
LEFT JOIN orders ON orders.id = stock_movements.order_id
WHERE stock_movements.id = $1;

-- Newest first by id, which follows the order the movements were made: the cursor
-- (before) is the id of the last movement of the previous page, so movements made while
-- someone pages through never shift a page. from is inclusive and until exclusive. A NULL
-- filter matches everything (stock-movements R10).
-- name: ListStockMovements :many
SELECT stock_movements.id, stock_movements.kind, stock_movements.product_id,
    products.name AS product_name, products.sku AS product_sku,
    stock_movements.change, stock_movements.stock_after, stock_movements.at,
    stock_movements.user_id, users.name AS user_name, stock_movements.note,
    stock_movements.order_id, orders.number AS order_number
FROM stock_movements
JOIN products ON products.id = stock_movements.product_id
JOIN users ON users.id = stock_movements.user_id
LEFT JOIN orders ON orders.id = stock_movements.order_id
WHERE (sqlc.narg(product_id)::bigint IS NULL OR stock_movements.product_id = sqlc.narg(product_id))
    AND (sqlc.narg(kind)::text IS NULL OR stock_movements.kind = sqlc.narg(kind))
    AND (sqlc.narg(from_at)::timestamptz IS NULL OR stock_movements.at >= sqlc.narg(from_at))
    AND (sqlc.narg(until_at)::timestamptz IS NULL OR stock_movements.at < sqlc.narg(until_at))
    AND (sqlc.narg(before)::bigint IS NULL OR stock_movements.id < sqlc.narg(before))
ORDER BY stock_movements.id DESC
LIMIT sqlc.arg(row_limit);
